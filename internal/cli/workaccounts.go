package cli

import (
	"slices"
	"strings"

	"github.com/flownative/minka-cli/internal/config"
	"github.com/flownative/minka-cli/internal/work"
)

// beatEveryAccount reports the list of a session in every account that holds
// one, each with the token of its own profile. A card is reported by the
// account it was taken up with, and the mod that runs the beat knows nothing of
// that: its environment is the one Claude Code was started with, not the one of
// the command that took the card up. --profile and --host narrow the beat to
// the account they name.
func (a *app) beatEveryAccount(session string) []signalled {
	if a.profileFlag != "" || a.hostFlag != "" {
		return []signalled{a.signal(session, 0)}
	}
	path, err := a.storePath()
	if err != nil || work.Untouched(path) {
		return []signalled{{reason: "nothing is remembered on this machine"}}
	}
	keys := a.accountsHolding(path, session)
	if len(keys) == 0 {
		return []signalled{a.signal(session, 0)}
	}
	beats := make([]signalled, 0, len(keys))
	for _, key := range keys {
		s, err := a.sessionForKey(key)
		if err != nil {
			beats = append(beats, signalled{reason: "not signed in with " + key, key: key})
			continue
		}
		beats = append(beats, a.signalWith(s, session, 0))
	}
	return beats
}

// accountsHolding are the accounts that hold a list of the session, sorted,
// without those no profile stands for any more.
func (a *app) accountsHolding(path, session string) []string {
	handle, err := a.openTheStore(path, 0, a.storeAccounts())
	if err != nil {
		return nil
	}
	defer handle.Close()
	var keys []string
	for key, account := range handle.File.Accounts {
		if _, held := account.Sessions[session]; held && handle.IsLive(key) {
			keys = append(keys, key)
		}
	}
	slices.Sort(keys)
	return keys
}

// sessionForKey signs in with the account the store keeps a list under: a
// profile by its own token, a host only by $MINKA_TOKEN where $MINKA_HOST names
// it. A host that no profile stands for has no token anywhere else.
func (a *app) sessionForKey(key string) (*session, error) {
	if config.CheckProfileName(key) == nil {
		path, file, err := a.loadConfig()
		if err != nil {
			return nil, err
		}
		c, err := profileChoice(file, path, key, chosenByStore)
		if err != nil {
			return nil, err
		}
		return a.sessionOf(c, path, file)
	}
	token := a.getenv("MINKA_TOKEN")
	host, err := config.NormalizeHost(a.getenv("MINKA_HOST"))
	if token == "" || err != nil || host != key {
		return nil, &Error{Code: CodeAuth, Message: "not signed in to " + key}
	}
	return a.sessionOf(choice{host: host, source: chosenByHostEnv, envToken: token}, "", nil)
}

// together is what several beats of one session say as one: sent where any
// went out, with every card and every note, and the reasons each with its
// account.
func together(beats []signalled) signalled {
	if len(beats) == 1 {
		return beats[0]
	}
	var all signalled
	var reasons []string
	for _, beat := range beats {
		all.sent = all.sent || beat.sent
		all.cards = append(all.cards, beat.cards...)
		all.letGo = append(all.letGo, beat.letGo...)
		if beat.reason != "" {
			reasons = append(reasons, beat.key+": "+beat.reason)
		}
	}
	all.reason = strings.Join(reasons, "; ")
	return all
}

// renameInTheStore moves the lists of a profile to its new name.
func (a *app) renameInTheStore(from, to string) error {
	path, err := a.storePath()
	if err != nil {
		return err
	}
	if work.Untouched(path) {
		return nil
	}
	handle, err := a.openStore(waitForTheStore)
	if err != nil {
		return err
	}
	defer handle.Close()
	if !handle.File.Rename(from, to) {
		return nil
	}
	if err := handle.Save(); err != nil {
		return &Error{Code: CodeConfig, Message: err.Error()}
	}
	return nil
}
