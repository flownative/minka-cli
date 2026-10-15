package cli

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/flownative/minka-cli/internal/api"
	"github.com/flownative/minka-cli/internal/cliupdate"
	"github.com/flownative/minka-cli/internal/config"
)

// cliUpdate is the field of an envelope that says a newer minka exists.
type cliUpdate struct {
	Current string `json:"current"`
	Latest  string `json:"latest,omitempty"`
	Minimum string `json:"minimum,omitempty"`
}

// noticeChannel is where a notice would be shown; each has its own day.
type noticeChannel int

const (
	noChannel noticeChannel = iota
	textChannel
	jsonChannel
)

// versionsSeen collects what the answers of this run said about the versions
// of minka. Requests may run side by side, hence the lock.
type versionsSeen struct {
	mu      sync.Mutex
	named   bool
	latest  string
	minimum string
}

// newClient is the client of a host, with its answers watched for the
// versions of minka Minka names.
func (a *app) newClient(host, token string) *api.Client {
	client := api.New(host, token, userAgent())
	client.ObserveAnswers(a.seen.observe)
	return client
}

// observe takes the versions an answer names. Minka sets the two headers
// each on its own, so an answer that names one of them says that the other is
// unset. An answer that names neither changes nothing: it may come from a
// Minka that names no versions at all.
func (v *versionsSeen) observe(header http.Header) {
	latest, latestNamed := cliupdate.FromHeader(header.Get(cliupdate.LatestHeader))
	minimum, minimumNamed := cliupdate.FromHeader(header.Get(cliupdate.MinimumHeader))
	if !latestNamed && !minimumNamed {
		return
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	v.named, v.latest, v.minimum = true, latest, minimum
}

func (v *versionsSeen) get() (named bool, latest, minimum string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.named, v.latest, v.minimum
}

// updateNoticeOff says whether $MINKA_NO_UPDATE_NOTICE switches the notice
// off, which any value but an empty one and 0 does.
func (a *app) updateNoticeOff() bool {
	value := a.getenv("MINKA_NO_UPDATE_NOTICE")
	return value != "" && value != "0"
}

// settleCLIUpdate remembers what this run learned about the versions of
// minka and says whether a notice is due on the channel given. It does its
// work once per run; a later call has nothing to add.
//
// Nothing here fails a command. A state that cannot be read counts as none,
// and a notice whose moment cannot be written is not shown, so that a
// directory nobody can write to does not bring the notice on every command.
func (a *app) settleCLIUpdate(channel noticeChannel) (cliupdate.State, *cliUpdate) {
	if a.updateSettled {
		return a.updateState, nil
	}
	a.updateSettled = true

	configPath, err := config.Path(a.getenv)
	if err != nil {
		return cliupdate.State{}, nil
	}
	path := cliupdate.Path(configPath)
	state, err := cliupdate.Read(path)
	changed := err != nil
	if err != nil {
		state = cliupdate.State{}
	}
	if named, latest, minimum := a.seen.get(); named && (state.Latest != latest || state.Minimum != minimum) {
		state.Latest, state.Minimum, changed = latest, minimum, true
	}

	var due *cliUpdate
	if channel != noChannel && !a.updateNoticeOff() {
		if standing := compareWith(a.ownVersion, state); standing.behind || standing.belowMinimum {
			shownAt := &state.TextNoticeAt
			if channel == jsonChannel {
				shownAt = &state.JSONNoticeAt
			}
			now := a.now()
			if shownAt.IsZero() || now.Sub(*shownAt) >= cliupdate.NoticeEvery || now.Before(*shownAt) {
				*shownAt = now.UTC()
				changed = true
				due = &cliUpdate{Current: a.ownVersionShown(), Latest: state.Latest, Minimum: state.Minimum}
			}
		}
	}

	if changed && cliupdate.Write(path, state) != nil {
		due = nil
	}
	a.updateState = state
	return state, due
}

// ownVersionShown is this minka's version the way Minka writes one: a release
// without the v of its tag, so that every line names versions alike. A build
// that is no release is shown as it calls itself.
func (a *app) ownVersionShown() string {
	if _, release := cliupdate.ParseOwn(a.ownVersion); release {
		return strings.TrimPrefix(a.ownVersion, "v")
	}
	return a.ownVersion
}

// standing is how this minka compares with what Minka names.
type standing struct {
	release      bool // this minka is a release and can be compared at all
	known        bool // Minka has named a latest or a minimum version
	behind       bool
	belowMinimum bool
}

// compareWith compares this minka with the versions in the state. Only a
// release is compared.
func compareWith(own string, state cliupdate.State) standing {
	var s standing
	current, release := cliupdate.ParseOwn(own)
	latest, latestKnown := cliupdate.ParseRelease(state.Latest)
	minimum, minimumKnown := cliupdate.ParseRelease(state.Minimum)
	s.release, s.known = release, latestKnown || minimumKnown
	if !release {
		return s
	}
	s.behind = latestKnown && current.Compare(latest) < 0
	s.belowMinimum = minimumKnown && current.Compare(minimum) < 0
	return s
}

// versionsNamed says which versions Minka names, for a line of doctor.
func versionsNamed(state cliupdate.State) string {
	var named []string
	if state.Latest != "" {
		named = append(named, "latest "+state.Latest)
	}
	if state.Minimum != "" {
		named = append(named, "minimum "+state.Minimum)
	}
	return strings.Join(named, ", ")
}

// writeUpdateNotice prints a notice on stderr, one line where a newer minka
// exists and two where this one is below the oldest Minka supports.
func writeUpdateNotice(w io.Writer, update *cliUpdate) {
	current, _ := cliupdate.ParseOwn(update.Current)
	if minimum, ok := cliupdate.ParseRelease(update.Minimum); ok && current.Compare(minimum) < 0 {
		fmt.Fprintf(w, "Warning: minka %s is older than %s, the oldest version Minka supports; commands may fail or miss features.\n", update.Current, update.Minimum)
		if update.Latest != "" {
			fmt.Fprintf(w, "Upgrade to %s with: brew upgrade minka\n", update.Latest)
		} else {
			fmt.Fprintln(w, "Upgrade with: brew upgrade minka")
		}
		return
	}
	fmt.Fprintf(w, "minka %s is available (this is %s). Upgrade with: brew upgrade minka\n", update.Latest, update.Current)
}

// doctorUpdateCheck says how this minka stands against what Minka named last.
// A newer version is no fault of the setup; being below the minimum is a
// warning, since commands may then fail.
func (a *app) doctorUpdateCheck() doctorCheck {
	check := doctorCheck{Name: "update", Status: checkOK}
	state, _ := a.settleCLIUpdate(noChannel)
	s := compareWith(a.ownVersion, state)
	own := a.ownVersionShown()
	switch {
	case !s.known:
		check.Detail = "no Minka has named a version of minka yet"
	case !s.release:
		check.Detail = "minka " + own + " is no release and is not compared; Minka names " + versionsNamed(state)
	case s.belowMinimum:
		check.Status = checkWarn
		check.Detail = "minka " + own + " is older than " + state.Minimum + ", the oldest version Minka supports"
		check.Hint = "Run: brew upgrade minka"
	case s.behind:
		check.Detail = "minka " + state.Latest + " is available, this is " + own
		check.Hint = "Run: brew upgrade minka"
	case state.Latest != "":
		check.Detail = "minka " + own + " is the latest Minka names"
	default:
		check.Detail = "minka " + own + " is not older than the oldest version Minka supports"
	}
	if a.updateNoticeOff() {
		check.Detail += "; the notice is switched off by $MINKA_NO_UPDATE_NOTICE"
	}
	return check
}
