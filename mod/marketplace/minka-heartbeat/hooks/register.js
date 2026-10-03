// Says from inside the running Claude Code process that its session is still
// there, so that the cards the session has in hand stay alight on the board.
// When the process ends, so do the beats, and the board forgets the cards three
// minutes after the last one.
//
// Everything about the cards is decided by minka heartbeat. This module only
// says when to ask, and whether the session is at work at that moment.

// The minka that wrote this mod, by the path it was started from.
const minka = 'minka'

// The server counts a signal for three minutes, and minka reports at most once
// a minute, so a beat every thirty seconds keeps a tile alight with room for a
// beat that is late.
const beatEvery = 30_000

// The tool calls that have started and not ended, of the main conversation and
// of every subagent alike. A subagent working in the background after the main
// turn has ended is at work all the same.
let toolsRunning = 0

let beating = false
// Whether a beat is owed after the one that is running, and whether that beat
// says somebody is at work; null where none is owed.
let owed = null

// Runs one heartbeat, and one more after it where a step came in meanwhile, so
// that the last step is always reported and beats never pile up behind a minka
// that is slow to answer.
//
// A beat that is not at work says that the process is alive and nothing more,
// so minka keeps such a session alight only for as long as it would keep any
// session alight after its last step.
async function beat($, atWork) {
  if (beating) {
    owed = owed === true || atWork
    return
  }
  beating = true
  let work = atWork
  try {
    for (;;) {
      try {
        // The session is asked on every beat, because /clear gives the process
        // a session of another name.
        const session = await $.session.id()
        const argv = work
          ? [minka, 'heartbeat', '--session', session, '--from-mod']
          : [minka, 'heartbeat', '--session', session, '--from-mod', '--idle']
        await $.process.run(argv, { timeoutMs: 15_000 })
      } catch {
        // A session that cannot be named, or a minka that cannot be started or
        // does not end, has nothing to say that the session could act on; the
        // next beat asks again.
      }
      if (owed === null) break
      work = owed
      owed = null
    }
  } finally {
    beating = false
  }
}

export function register(on) {
  on('session.start', async ($, e, next) => {
    // A tool that runs for an hour is work for the hour; between steps the
    // timer only says that the process is alive.
    $.clock.every(beatEvery, () => beat($, toolsRunning > 0))
    beat($, false)
    return next(e)
  })

  on('turn.start', async ($, e, next) => {
    beat($, true)
    return next(e)
  })

  // The beat after the tool has run reports what the tool changed, such as a
  // card that minka work start has just taken up.
  on('tool.call', async ($, e, next) => {
    toolsRunning++
    try {
      return await next(e)
    } finally {
      toolsRunning--
      beat($, true)
    }
  })
}
