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
