import { expect, mock, test } from 'claude-code/testing'

// Lets the beats that a hook started without waiting for them run to their end.
async function settle(clock: { settle: () => Promise<void> }) {
  for (let i = 0; i < 20; i++) {
    await clock.settle()
    await new Promise((resolve) => setTimeout(resolve, 0))
  }
}

type Stubs = {
  // What a run of minka waits for before it answers.
  minkaAnswers?: () => Promise<unknown>
  // What a tool waits for before it answers.
  toolAnswers?: () => Promise<unknown>
  // What asking for the session does.
  sessionId?: () => unknown
}

function stubTheSession(on: any, ran: string[][], stubs: Stubs = {}) {
  on('session.start', () => ({ cwd: '/work' }))
  on('turn.start', ($: any, e: any) => ({ turnId: e.turnId }))
  on('tool.call', async () => {
    await stubs.toolAnswers?.()
    return { result: 'ok' }
  })
  on('session.id', stubs.sessionId ?? (() => ({ value: 'the-session' })))
  on('process.run', async ($: any, e: any) => {
    // The program is whichever minka wrote the mod; what it is told is the same.
    ran.push(e.argv.slice(1))
    await stubs.minkaAnswers?.()
    return { value: { exitCode: 0, stdout: '', stderr: '' } }
  })
}

const atWork = ['heartbeat', '--session', 'the-session', '--from-mod']
const idle = [...atWork, '--idle']

test('a step is work, and the timer is work only while a tool runs', async ($, on) => {
  const clock = mock.clock(on)
  const ran: string[][] = []
  stubTheSession(on, ran, { toolAnswers: () => clock.sleep(65_000) })

  await $.session.start({ surface: 'terminal', isInteractive: true, cwd: '/work' })
  await settle(clock)
  expect(ran).toEqual([idle])

  await $.turn.start({ text: 'go', turnId: 't1' })
  await settle(clock)
  expect(ran.slice(1)).toEqual([atWork])

  // A tool that runs for over a minute: the timer beats while it runs, and the
  // step that ends it beats as well.
  const running = $.tool.call({ tool: 'Bash', command: 'make test' })
  await settle(clock)
  await clock.advance(30_000)
  await settle(clock)
  await clock.advance(30_000)
  await settle(clock)
  expect(ran.slice(2)).toEqual([atWork, atWork])
  await clock.advance(5_000)
  await running
  await settle(clock)
  expect(ran.slice(4)).toEqual([atWork])

  // No tool runs any more: the timer only says the process is alive.
  await clock.advance(30_000)
  await settle(clock)
  expect(ran.slice(5)).toEqual([idle])
})

test('a tool call outside a turn, such as one of a subagent in the background, is work', async ($, on) => {
  const clock = mock.clock(on)
  const ran: string[][] = []
  stubTheSession(on, ran)

  await $.session.start({ surface: 'terminal', isInteractive: true, cwd: '/work' })
  await settle(clock)
  ran.length = 0

  await $.tool.call({ tool: 'Bash', command: 'ls', agentId: 'a1' })
  await settle(clock)
  expect(ran).toEqual([atWork])
})

test('a turn that never ends does not count as work for ever', async ($, on) => {
  const clock = mock.clock(on)
  const ran: string[][] = []
  stubTheSession(on, ran)

  await $.session.start({ surface: 'terminal', isInteractive: true, cwd: '/work' })
  await $.turn.start({ text: 'go', turnId: 'lost' })
  await settle(clock)
  ran.length = 0

  await clock.advance(30_000)
  await settle(clock)
  expect(ran).toEqual([idle])
})

test('beats do not pile up behind a minka that is slow to answer, and the one owed is work', async ($, on) => {
  const clock = mock.clock(on)
  const ran: string[][] = []
  stubTheSession(on, ran, { minkaAnswers: () => clock.sleep(10_000) })

  await $.session.start({ surface: 'terminal', isInteractive: true, cwd: '/work' })
  await settle(clock)
  for (let i = 0; i < 5; i++) await $.tool.call({ tool: 'Read', file_path: 'a' })
  await settle(clock)
  expect(ran).toEqual([idle])

  await clock.advance(10_000)
  await settle(clock)
  expect(ran).toEqual([idle, atWork])
})

test('the beat owed behind a slow one is work where a step it stands for was, even if the timer came after it', async ($, on) => {
  const clock = mock.clock(on)
  const ran: string[][] = []
  stubTheSession(on, ran, { minkaAnswers: () => clock.sleep(40_000) })

  await $.session.start({ surface: 'terminal', isInteractive: true, cwd: '/work' })
  await settle(clock)
  await $.tool.call({ tool: 'Read', file_path: 'a' })
  await settle(clock)
  await clock.advance(30_000)
  await settle(clock)
  expect(ran).toEqual([idle])

  await clock.advance(10_000)
  await settle(clock)
  expect(ran).toEqual([idle, atWork])
})

test('a tool call that never returns is work for two hours and no longer', async ($, on) => {
  const clock = mock.clock(on)
  const ran: string[][] = []
  stubTheSession(on, ran, { toolAnswers: () => new Promise(() => {}) })

  // The call starts before the timer does, so that the timer has to run only
  // through the last minute of the two hours and not through all of them.
  $.tool.call({ tool: 'mcp__hung__call' }).catch(() => {})
  await settle(clock)
  await clock.set(2 * 60 * 60 * 1000 - 60_000)
  await $.session.start({ surface: 'terminal', isInteractive: true, cwd: '/work' })
  await settle(clock)
  expect(ran).toEqual([idle])

  await clock.advance(30_000)
  await settle(clock)
  expect(ran.slice(1)).toEqual([atWork])

  await clock.advance(30_000)
  await settle(clock)
  await clock.advance(30_000)
  await settle(clock)
  expect(ran.slice(2)).toEqual([idle, idle])
})

// Mods above the heartbeat that give up on a tool call, after a second or at
// once, which abandons the call beneath them as an interrupt does. The kit
// loads a mod from its source, so each one names its own time.
const impatient = {
  name: 'impatient',
  tier: 'prepend',
  register(on: any) {
    on('tool.call', async ($: any, e: any, next: any) => {
      await Promise.race([next(e), $.clock.sleep(1000)])
      return { result: 'given up' }
    })
  },
}
const hasty = {
  name: 'hasty',
  tier: 'prepend',
  register(on: any) {
    // The first question for the time is answered only once the clock has
    // moved on, so that the call is abandoned while the hook beneath waits for
    // the answer.
    let first = true
    on('clock.now', async ($: any, e: any, next: any) => {
      if (first) {
        first = false
        await $.clock.sleep(1)
      }
      return next(e)
    })
    on('tool.call', async ($: any, e: any, next: any) => {
      await Promise.race([next(e), $.clock.sleep(0)])
      return { result: 'given up' }
    })
  },
}

test('an abandoned tool call is no longer work', { plugins: [impatient] }, async ($, on) => {
  const clock = mock.clock(on)
  const ran: string[][] = []
  stubTheSession(on, ran, { toolAnswers: () => new Promise(() => {}) })

  await $.session.start({ surface: 'terminal', isInteractive: true, cwd: '/work' })
  await settle(clock)
  const pending = $.tool.call({ tool: 'mcp__hung__call' })
  await settle(clock)
  await clock.advance(1000)
  expect(await pending).toEqual({ result: 'given up' })
  await settle(clock)
  await clock.advance(30_000)
  await settle(clock)
  expect(ran.slice(1)).toEqual([idle])
})

test('a tool call abandoned while the time of its start is asked for is no work', { plugins: [hasty] }, async ($, on) => {
  const clock = mock.clock(on)
  const ran: string[][] = []
  stubTheSession(on, ran, { toolAnswers: () => new Promise(() => {}) })

  await $.session.start({ surface: 'terminal', isInteractive: true, cwd: '/work' })
  await settle(clock)
  const pending = $.tool.call({ tool: 'mcp__hung__call' })
  await settle(clock)
  expect(await pending).toEqual({ result: 'given up' })
  await clock.advance(30_000)
  await settle(clock)
  expect(ran.slice(1)).toEqual([idle])
})

test('a session that cannot be named costs one beat and not the timer', async ($, on) => {
  const clock = mock.clock(on)
  const ran: string[][] = []
  let named = false
  stubTheSession(on, ran, {
    sessionId: () => {
      if (!named) {
        named = true
        return { deny: 'not now' }
      }
      return { value: 'the-session' }
    },
  })

  await $.session.start({ surface: 'terminal', isInteractive: true, cwd: '/work' })
  await settle(clock)
  expect(ran).toEqual([])

  await clock.advance(30_000)
  await settle(clock)
  expect(ran).toEqual([idle])
})
