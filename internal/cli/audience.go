package cli

import "github.com/spf13/cobra"

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
` + personHelp + ` A name or an e-mail address
is looked up among the members of the project the tool stands in. A person
named more than once is told once.`

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
		return "Somebody named with --notify is not a member of the project. minka people list <projectId> gives the members.", true
	}
	return "", false
}
