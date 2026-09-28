package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// What Minka answers a new document or post whose audience and people do not
// go together, or that names somebody who cannot be told.
const (
	apiCodeNotifyIDsMissing               = "notifyIdsMissing"
	apiCodeNotifyIDsWithoutChosenAudience = "notifyIdsWithoutChosenAudience"
	apiCodePersonCannotSeeIt              = "personCannotSeeIt"
	apiCodePersonNotAProjectMember        = "personNotAProjectMember"
)

// audienceHelp says what --audience and --notify take, for whatever a new
// document or post reaches.
const audienceHelp = `--audience says whom it reaches: "everybody" in the project, "chosen" with
one --notify per person, or "nobody". It is required. --notify belongs to
"chosen" alone: Minka refuses names under any other audience with
notifyIdsWithoutChosenAudience and "chosen" without a name with
notifyIdsMissing; this command sends what it is given and leaves the rule
to Minka.
A person is given by ID, or as "me"; minka people list <projectId> gives the
IDs.`

const namedTwiceHelp = `Minka refuses a person named twice as one who is not a member of the project
(personNotAProjectMember); you yourself may be named as often as you like.`

// checkAudience holds the one word a new document or post cannot be written
// without. Which people go with which audience is the server's rule and is
// left to it, so that there is one place where it stands.
func checkAudience(cmd *cobra.Command, what, audience string) error {
	switch {
	case audience == "":
		return usageErrorf(cmd, "say whom the %s reaches: --audience everybody, chosen or nobody", what)
	case audience != "everybody" && audience != "chosen" && audience != "nobody":
		return usageErrorf(cmd, "--audience is everybody, chosen or nobody, not %q", audience)
	}
	return nil
}

// notifiedPeople is the people a new document or post tells about itself. Only
// the identifier and "me" are read here: which project a tool hangs in is not
// known before what is written in it exists, and a name can only be looked up
// among the members of a project.
func (a *app) notifiedPeople(cmd *cobra.Command, s *session, notify []string) ([]string, error) {
	var people []string
	for _, value := range notify {
		if strings.EqualFold(strings.TrimSpace(value), "me") {
			id, err := a.meID(s)()
			if err != nil {
				return nil, err
			}
			people = append(people, id)
			continue
		}
		if err := requireID(cmd, "--notify", value); err != nil {
			return nil, &Error{
				Code:    CodeUsage,
				Message: fmt.Sprintf("--notify takes the ID of a person, or me; %q is neither", value),
				Hint:    "minka people list <projectId> gives the ID of every member of the project.",
			}
		}
		people = append(people, value)
	}
	return people, nil
}

// audienceHint says what to change where Minka refused the people a request
// names. Whether somebody may see what was written depends on the kind of
// thing, so that refusal is left to the caller.
func audienceHint(apiCode string) (string, bool) {
	switch apiCode {
	case apiCodeNotifyIDsMissing:
		return "An audience of \"chosen\" tells the people it names, and this request named none. Give one --notify per person, or reach everybody or nobody with --audience.", true
	case apiCodeNotifyIDsWithoutChosenAudience:
		return "Only --audience chosen tells the people a request names. Leave the named people out, or say --audience chosen to tell them.", true
	case apiCodePersonNotAProjectMember:
		return "Somebody named with --notify is not a member of the project, or somebody other than you is named twice, which Minka refuses the same way. minka people list <projectId> gives the members.", true
	}
	return "", false
}
