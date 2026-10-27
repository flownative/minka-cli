package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"text/tabwriter"
	"unicode"

	"github.com/flownative/minka-cli/internal/api"
	"github.com/spf13/cobra"
	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

// labelColors are the colours a label takes, in Minka's order. They feed the
// hint once Minka has refused a colour, never a refusal of its own.
var labelColors = []string{"red", "orange", "yellow", "lime", "green", "teal", "blue", "indigo", "purple", "pink", "brown", "gray"}

var labelColorHint = "A label takes one of " + strings.Join(labelColors, ", ") + ", spelt exactly so."

const (
	apiCodeLabelNameMissing    = "labelNameMissing"
	apiCodeLabelNameTooLong    = "labelNameTooLong"
	apiCodeLabelNameMalformed  = "labelNameMalformed"
	apiCodeLabelColorUnknown   = "labelColorUnknown"
	apiCodeLabelNameTaken      = "labelNameTaken"
	apiCodeLabelPlaceUnknown   = "labelPlaceUnknown"
	apiCodeLabelUnknown        = "labelUnknown"
	apiCodeLabelsShareAScope   = "labelsShareAScope"
	apiCodeLabelScopeMissing   = "labelScopeMissing"
	apiCodeLabelScopeMalformed = "labelScopeMalformed"

	labelNameRules = `A label name holds letters, digits, spaces, "-", "+" and "*", at most 50
characters, with at most one "::" between a scope and a value, such as
"Priority::High". Spaces around "::" and runs of spaces are tidied away.`

	labelPlaceHelp = `The place is given by exactly one of --project, --board and --tool (a Docs &
Files tool). A label of a board or a tool is reached under that board or tool
only, a label of the project under the project only.`

	labelArgHelp = `<label> is the label's ID or its name. A name is looked up in the labels of
the place the way Minka compares names: case and spacing aside, so
"priority :: high" finds "Priority::High".`

	labelWritersHelp = `Only the team of a project reads and writes its labels: a client of the project
gets exit 2 (not_found) like somebody outside it.`

	labelItemHelp = `Each <label> is a label ID or a name. An ID goes to Minka as it is; a name is
looked up by Minka among the labels of the item's own board or tool and of its
project, case and spacing aside. Several labels go in one request each, in the
order given; the first refusal stops the run, and the error says which labels
were applied before it ("applied" in data). On success the answer is every
label the item carries afterwards.`

	newCardLabelHelp = `--label puts a label of the board or of its project on the card and can be
given more than once; the card is created with its labels, or not at all. A
label is its ID or its name: a name is looked up in "minka labels list --board"
the way Minka compares names, case and spacing aside, and one found nowhere
exits 2 (not_found) before anything is written. A label named twice goes once.
A card takes one label of each scope: two such as "Priority::High" and
"Priority::Low" exit 9 (validation) with meta.apiCode labelsShareAScope, and a
label of another board, tool or project with labelUnknown.`

	labelScopeRenameMergeHelp = `Renaming into a scope already in use joins the two: "cull::select" renamed to
"auswahl" stands beside an existing "auswahl::keep". Only a resulting name that
exists already anywhere in the project, such as "auswahl::select", refuses the
rename: exit 9 (validation) with meta.apiCode labelNameTaken, and no label is
renamed. An item that carried a label of each scope keeps both until it takes
the next label of that scope, which replaces them.`

	labelScopeHelp = `A scoped label such as "Priority::High" takes any other label of the same
scope off the item: putting on "Priority::Low" replaces it. Plain labels never
replace one another.`
)

// labelRefusal classifies a failed write of labels and names the way on.
func labelRefusal(s *session, err error) *Error {
	e := s.fail(err)
	switch e.APICode {
	case apiCodeLabelNameMissing, apiCodeLabelNameTooLong, apiCodeLabelNameMalformed:
		e.Hint = strings.ReplaceAll(labelNameRules, "\n", " ")
	case apiCodeLabelColorUnknown:
		e.Hint = labelColorHint
	case apiCodeLabelNameTaken:
		e.Hint = "Label names are unique across the project and all its boards and tools, case aside. List them with: minka labels list --project <projectId>"
	case apiCodeLabelPlaceUnknown:
		e.Hint = "A label moves only among the other labels of its own place, which \"minka labels list\" with the same place flag shows; or give --first."
	case apiCodeLabelUnknown:
		e.Hint = "An item takes the labels of its own board or Docs & Files tool and of its project. List them with: minka labels list --board <boardId> or --tool <toolId>"
	default:
		if e.Code == CodeNotFound {
			e.Hint = "It does not exist, or belongs to a project you are not on the team of."
		}
	}
	return e
}

// labelPlaceFlags are the three ways of naming where labels are kept.
type labelPlaceFlags struct {
	project, board, tool string
}

func (p *labelPlaceFlags) register(cmd *cobra.Command) {
	flags := cmd.Flags()
	flags.StringVar(&p.project, "project", "", "ID of the project whose own labels are meant")
	flags.StringVar(&p.board, "board", "", "ID of the board whose own labels are meant")
	flags.StringVar(&p.tool, "tool", "", "ID of the Docs & Files tool whose own labels are meant")
}

// place checks that exactly one place is given and that it is an ID.
func (p *labelPlaceFlags) place(cmd *cobra.Command) (api.LabelPlace, error) {
	given := 0
	for _, value := range []string{p.project, p.board, p.tool} {
		if value != "" {
			given++
		}
	}
	if given != 1 {
		return api.LabelPlace{}, usageErrorf(cmd, "say where the labels are: give exactly one of --project, --board and --tool")
	}
	switch {
	case p.project != "":
		if err := requireID(cmd, "--project", &p.project); err != nil {
			return api.LabelPlace{}, err
		}
		return api.LabelPlace{Kind: "projects", ID: p.project}, nil
	case p.board != "":
		if err := requireID(cmd, "--board", &p.board); err != nil {
			return api.LabelPlace{}, err
		}
		return api.LabelPlace{Kind: "boards", ID: p.board}, nil
	}
	if err := requireID(cmd, "--tool", &p.tool); err != nil {
		return api.LabelPlace{}, err
	}
	return api.LabelPlace{Kind: "files-tools", ID: p.tool}, nil
}

// placeName names a place in a sentence.
func placeName(place api.LabelPlace) string {
	switch place.Kind {
	case "projects":
		return "project " + place.ID
	case "boards":
		return "board " + place.ID
	}
	return "Docs & Files tool " + place.ID
}

// placeFlag is the flag that names a place again, for a hint or a breadcrumb.
func placeFlag(place api.LabelPlace) string {
	switch place.Kind {
	case "projects":
		return "--project " + place.ID
	case "boards":
		return "--board " + place.ID
	}
	return "--tool " + place.ID
}

// ownsLabel says whether a label is one of the place's own, which is what
// the addresses of a label under that place reach.
func ownsLabel(place api.LabelPlace, label api.Label) bool {
	if place.Kind == "projects" {
		return label.ToolID == nil && label.ProjectID == place.ID
	}
	return label.ToolID != nil && *label.ToolID == place.ID
}

var labelScopeSeparator = regexp.MustCompile(` ?:: ?`)

// labelNameKey is what Minka compares two label names by: composed, runs of
// white space as one space, no space around "::", trimmed, case-folded.
func labelNameKey(name string) string {
	collapsed := strings.Join(strings.FieldsFunc(norm.NFC.String(name), unicode.IsSpace), " ")
	return cases.Fold().String(strings.TrimSpace(labelScopeSeparator.ReplaceAllString(collapsed, "::")))
}

// resolveLabel finds one of the place's own labels by its ID or its name. An
// ID is taken as it is and read nowhere; a name is looked up in the list of
// the place.
//
// neighbourFlag names the flag of a neighbour of a move, empty for the label
// itself: a neighbour has to be one of the same place, and a label of the
// project is no way on there.
func (a *app) resolveLabel(s *session, place api.LabelPlace, value, neighbourFlag string, labels *[]api.Label) (string, *api.Label, error) {
	if id, isID := asID(value); isID {
		return id, nil, nil
	}
	if *labels == nil {
		listed, err := s.client.Labels(a.ctx, place)
		if err != nil {
			return "", nil, labelRefusal(s, err)
		}
		*labels = listed.Value
		if *labels == nil {
			*labels = []api.Label{}
		}
	}
	key := labelNameKey(value)
	for i, label := range *labels {
		if labelNameKey(label.Name) != key {
			continue
		}
		if ownsLabel(place, label) {
			return label.ID, &(*labels)[i], nil
		}
		if neighbourFlag != "" {
			return "", nil, &Error{Code: CodeNotFound,
				Message: fmt.Sprintf("%s %q is a label of project %s, not of %s", neighbourFlag, label.Name, label.ProjectID, placeName(place)),
				Hint:    "A label moves only among the labels of its own place; list them with: minka labels list " + placeFlag(place)}
		}
		return "", nil, &Error{Code: CodeNotFound,
			Message: fmt.Sprintf("label %q belongs to project %s, not to %s", label.Name, label.ProjectID, placeName(place)),
			Hint:    "Give --project " + label.ProjectID + " to change a label of the project."}
	}
	return "", nil, &Error{Code: CodeNotFound,
		Message: fmt.Sprintf("%s has no label %q", placeName(place), value),
		Hint:    "List its labels with: minka labels list " + placeFlag(place)}
}

// checkLabelValues refuses an empty label name before anything is sent: it
// names no label.
func checkLabelValues(cmd *cobra.Command, values []string) error {
	for _, value := range values {
		if strings.TrimFunc(value, unicode.IsSpace) == "" {
			return usageErrorf(cmd, "a label is named by its ID or its name, and an empty name names none")
		}
	}
	return nil
}

// labelsForANewCard turns the labels given for a new card into IDs, each
// once. An ID goes as it is; a name is looked up among the labels the cards
// of the board can take, its own and the project's, which are read once and
// only for a name. Two labels of one scope are left to Minka to refuse.
func (a *app) labelsForANewCard(s *session, boardID string, values []string) ([]string, error) {
	var ids []string
	var labels []api.Label
	place := api.LabelPlace{Kind: "boards", ID: boardID}
	for _, value := range values {
		id, isID := asID(value)
		if !isID {
			if labels == nil {
				listed, err := s.client.Labels(a.ctx, place)
				if err != nil {
					return nil, labelRefusal(s, err)
				}
				labels = append([]api.Label{}, listed.Value...)
			}
			label, err := labelNamedOnABoard(labels, boardID, value)
			if err != nil {
				return nil, err
			}
			id = label.ID
		}
		if !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

// labelNamedOnABoard is the one label of the list a name names, compared the
// way Minka compares names.
func labelNamedOnABoard(labels []api.Label, boardID, name string) (api.Label, error) {
	key := labelNameKey(name)
	var matches []api.Label
	for _, label := range labels {
		if labelNameKey(label.Name) == key {
			matches = append(matches, label)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return api.Label{}, &Error{Code: CodeNotFound,
			Message: fmt.Sprintf("neither board %s nor its project has a label %q", boardID, name),
			Hint:    "A card takes the labels of its board and of its project. List them with: minka labels list --board " + boardID}
	}
	var candidates []string
	for _, label := range matches {
		candidates = append(candidates, label.Name+" ("+label.ID+")")
	}
	return api.Label{}, &Error{Code: CodeAmbiguous,
		Message: fmt.Sprintf("%q matches %d labels", name, len(matches)),
		Hint:    "Give the label's ID. Candidates: " + strings.Join(candidates, ", ")}
}

// newCardLabelRefusal names the way on where Minka refused the labels of a
// new card, and leaves every other refusal as it is.
func newCardLabelRefusal(e *Error, boardID string) *Error {
	switch e.APICode {
	case apiCodeLabelsShareAScope:
		e.Hint = "A card takes one label of each scope, the part before \"::\": give only one of them. The scopes of the board's labels show in: minka labels list --board " + boardID
	case apiCodeLabelUnknown:
		e.Hint = "A card takes the labels of its board and of its project. List them with: minka labels list --board " + boardID
	}
	return e
}

func labelBreadcrumbs(place api.LabelPlace, labelID string) []Breadcrumb {
	crumbs := []Breadcrumb{{Action: "list", Cmd: "minka labels list " + placeFlag(place), Description: "List the labels of the place"}}
	if labelID != "" {
		crumbs = append(crumbs, Breadcrumb{Action: "edit", Cmd: "minka labels edit " + labelID + " " + placeFlag(place) + " --name <name>", Description: "Rename or recolour the label"})
	}
	return crumbs
}

func (a *app) newLabelsCmd() *cobra.Command {
	return group("labels", "List, create, rename, recolour, move and delete the labels of a project, board or Docs & Files tool",
		a.newLabelsListCmd(),
		a.newLabelsCreateCmd(),
		a.newLabelsEditCmd(),
		a.newLabelsRenameScopeCmd(),
		a.newLabelsMoveCmd(),
		a.newLabelsDeleteCmd(),
	)
}

func (a *app) newLabelsListCmd() *cobra.Command {
	var placeFlags labelPlaceFlags
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List the labels the items of a place can take",
		Long: `Lists the labels of a project, or of a board or Docs & Files tool: under a board
or a tool, its own labels first and then the project's, which its items can
take as well. position counts from 1 within each place.

` + labelPlaceHelp + `

An archived project still lists its labels. ` + labelWritersHelp,
		Example: `  minka labels list --board 0199...
  minka labels list --project 0199... --json`,
		Args: requireArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			place, err := placeFlags.place(cmd)
			if err != nil {
				return err
			}
			s, err := a.session()
			if err != nil {
				return err
			}
			labels, err := s.client.Labels(a.ctx, place)
			if err != nil {
				return labelRefusal(s, err)
			}
			return a.ok(success{
				data:    labels.Raw,
				summary: plural(len(labels.Value), "label", "labels") + " in " + placeName(place),
				breadcrumbs: []Breadcrumb{
					{Action: "create", Cmd: "minka labels create <name> " + placeFlag(place), Description: "Add a label to the place"},
				},
				text: func(w io.Writer) { renderLabels(w, labels.Value) },
			})
		},
	}
	placeFlags.register(cmd)
	return cmd
}

func (a *app) newLabelsCreateCmd() *cobra.Command {
	var placeFlags labelPlaceFlags
	var color string
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Add a label at the end of a project, board or Docs & Files tool",
		Long: `Adds a label at the end of the labels of a place. Without --color Minka gives
it the colour it suggests: one that stands out from the labels in reach.

` + labelNameRules + ` Names are unique across the project and all its
boards and tools, case aside: a name taken exits 9 (validation) with
meta.apiCode labelNameTaken. A refused name exits 9 with labelNameMissing,
labelNameTooLong or labelNameMalformed.

--color takes one of ` + strings.Join(labelColors, ", ") + `,
spelt exactly so. Minka refuses any other value: exit 9 (validation) with
meta.apiCode labelColorUnknown.

` + labelPlaceHelp + `

` + labelWritersHelp,
		Example: `  minka labels create "Priority::High" --board 0199... --color red
  minka labels create Blocked --project 0199... --json`,
		Args: requireArgs("name"),
		RunE: func(cmd *cobra.Command, args []string) error {
			place, err := placeFlags.place(cmd)
			if err != nil {
				return err
			}
			input := api.LabelInput{Name: &args[0]}
			if cmd.Flags().Changed("color") {
				input.Color = &color
			}
			s, err := a.session()
			if err != nil {
				return err
			}
			label, err := s.client.CreateLabel(a.ctx, place, input)
			if err != nil {
				return labelRefusal(s, err)
			}
			summary := fmt.Sprintf("Added label %s in %s to %s", label.Value.Name, label.Value.Color, placeName(place))
			return a.ok(success{
				data:        label.Raw,
				summary:     summary,
				breadcrumbs: labelBreadcrumbs(place, label.Value.ID),
				text:        func(w io.Writer) { fmt.Fprintf(w, "%s.\nLabel: %s\n", summary, label.Value.ID) },
			})
		},
	}
	placeFlags.register(cmd)
	cmd.Flags().StringVar(&color, "color", "", "Colour of the label, one of Minka's twelve (see above); Minka suggests one without it")
	return cmd
}

func (a *app) newLabelsEditCmd() *cobra.Command {
	var placeFlags labelPlaceFlags
	var name, color string
	cmd := &cobra.Command{
		Use:   "edit <label>",
		Short: "Rename or recolour a label",
		Long: `Gives a label a new name, a new colour or both; at least one of --name and
--color is required. What is not given stays as it is. A rename that changes
only the case is allowed. Renaming a label into a scope takes nothing off the
items: one may then carry two labels of the same scope.

` + labelNameRules + `

` + labelArgHelp + `

` + labelPlaceHelp + `

` + labelWritersHelp,
		Example: `  minka labels edit "Priority::High" --board 0199... --color orange
  minka labels edit 0199... --project 0199... --name "Priority::Urgent"`,
		Args: requireArgs("label"),
		RunE: func(cmd *cobra.Command, args []string) error {
			var input api.LabelInput
			if cmd.Flags().Changed("name") {
				input.Name = &name
			}
			if cmd.Flags().Changed("color") {
				input.Color = &color
			}
			if input.Name == nil && input.Color == nil {
				return usageErrorf(cmd, "nothing to change, give --name or --color")
			}
			place, err := placeFlags.place(cmd)
			if err != nil {
				return err
			}
			s, err := a.session()
			if err != nil {
				return err
			}
			var labels []api.Label
			labelID, _, err := a.resolveLabel(s, place, args[0], "", &labels)
			if err != nil {
				return err
			}
			label, err := s.client.UpdateLabel(a.ctx, place, labelID, input)
			if err != nil {
				return labelRefusal(s, err)
			}
			summary := fmt.Sprintf("Changed label %s: %s, %s", label.Value.ID, label.Value.Name, label.Value.Color)
			return a.ok(success{
				data:        label.Raw,
				summary:     summary,
				breadcrumbs: labelBreadcrumbs(place, ""),
				text:        func(w io.Writer) { fmt.Fprintf(w, "%s.\n", summary) },
			})
		},
	}
	placeFlags.register(cmd)
	cmd.Flags().StringVar(&name, "name", "", "New name of the label")
	cmd.Flags().StringVar(&color, "color", "", "New colour of the label, one of Minka's twelve (see above)")
	return cmd
}

func (a *app) newLabelsMoveCmd() *cobra.Command {
	var placeFlags labelPlaceFlags
	var after, before string
	var first bool
	cmd := &cobra.Command{
		Use:   "move <label>",
		Short: "Move a label to another place among the labels of its place",
		Long: `Puts a label elsewhere among the labels of its own place: --first at the front,
--after right behind the label named, --before right in front of it. Exactly
one of the three is required. The neighbour has to be another label of the
same place: one of another place exits 9 (validation) with meta.apiCode
labelPlaceUnknown, the label itself exits 1 (usage) before anything is written.
The answer is the moved label with its new position.

` + labelArgHelp + ` The neighbour is given the same way.

` + labelPlaceHelp + `

` + labelWritersHelp,
		Example: `  minka labels move Blocked --board 0199... --first
  minka labels move "Priority::Low" --board 0199... --after "Priority::High"`,
		Args: requireArgs("label"),
		RunE: func(cmd *cobra.Command, args []string) error {
			placements := 0
			for _, given := range []bool{first, after != "", before != ""} {
				if given {
					placements++
				}
			}
			if placements != 1 {
				return usageErrorf(cmd, "say where the label goes: give exactly one of --first, --after and --before")
			}
			place, err := placeFlags.place(cmd)
			if err != nil {
				return err
			}
			s, err := a.session()
			if err != nil {
				return err
			}
			var labels []api.Label
			labelID, _, err := a.resolveLabel(s, place, args[0], "", &labels)
			if err != nil {
				return err
			}
			// Only the one neighbour named is sent: where both are, Minka
			// places by the previous one alone.
			var input api.MoveLabelInput
			for _, neighbour := range []struct {
				flag, value string
				id          *string
			}{{"--after", after, &input.PreviousLabelID}, {"--before", before, &input.NextLabelID}} {
				if neighbour.value == "" {
					continue
				}
				id, _, err := a.resolveLabel(s, place, neighbour.value, neighbour.flag, &labels)
				if err != nil {
					return err
				}
				if id == labelID {
					return usageErrorf(cmd, "%s names the label that is to move; name another label of %s", neighbour.flag, placeName(place))
				}
				*neighbour.id = id
			}
			label, err := s.client.MoveLabel(a.ctx, place, labelID, input)
			if err != nil {
				return labelRefusal(s, err)
			}
			summary := fmt.Sprintf("Moved label %s to position %d of %s", label.Value.Name, label.Value.Position, placeName(place))
			return a.ok(success{
				data:        label.Raw,
				summary:     summary,
				breadcrumbs: labelBreadcrumbs(place, ""),
				text:        func(w io.Writer) { fmt.Fprintf(w, "%s.\n", summary) },
			})
		},
	}
	placeFlags.register(cmd)
	cmd.Flags().BoolVar(&first, "first", false, "Put the label first in its place")
	cmd.Flags().StringVar(&after, "after", "", "Put the label right behind this label (ID or name) of the same place")
	cmd.Flags().StringVar(&before, "before", "", "Put the label right in front of this label (ID or name) of the same place")
	return cmd
}

func (a *app) newLabelsRenameScopeCmd() *cobra.Command {
	var placeFlags labelPlaceFlags
	cmd := &cobra.Command{
		Use:   "rename-scope <scope> <newScope>",
		Short: "Rename the scope of every label of that scope at a place",
		Long: `Gives every label of <scope> among the place's own labels the scope
<newScope>: with "Priority" renamed to "Urgency", "Priority::High" becomes
"Urgency::High". Labels keep their colour, their position and the items that
carry them; the project's labels are not renamed under a board or a tool, nor
the other way round. Scopes compare case aside, and <newScope> takes the case
it is written in, so renaming a scope into itself changes only its case.

` + labelScopeRenameMergeHelp + `

A scope follows the rules of one side of a name and holds no "::": an empty one
exits 9 (validation) with meta.apiCode labelScopeMissing, a malformed one with
labelScopeMalformed, and a resulting name over 50 characters with
labelNameTooLong. A scope that no label of the place has exits 2 (not_found).
The answer is the place's own labels afterwards, without the project's.

` + labelPlaceHelp + `

` + labelWritersHelp,
		Example: `  minka labels rename-scope Priority Urgency --board 0199...
  minka labels rename-scope cull auswahl --project 0199... --json`,
		Args: requireArgs("scope", "newScope"),
		RunE: func(cmd *cobra.Command, args []string) error {
			place, err := placeFlags.place(cmd)
			if err != nil {
				return err
			}
			s, err := a.session()
			if err != nil {
				return err
			}
			labels, err := s.client.RenameLabelScope(a.ctx, place, api.RenameLabelScopeInput{Scope: args[0], NewScope: args[1]})
			if err != nil {
				return scopeRenameRefusal(s, err, place, args[0])
			}
			summary := fmt.Sprintf("Renamed scope %s to %s in %s", args[0], args[1], placeName(place))
			return a.ok(success{
				data:        labels.Raw,
				summary:     summary,
				breadcrumbs: labelBreadcrumbs(place, ""),
				text: func(w io.Writer) {
					fmt.Fprintf(w, "%s.\n", summary)
					renderLabels(w, labels.Value)
				},
			})
		},
	}
	placeFlags.register(cmd)
	return cmd
}

// scopeRenameRefusal classifies a refused rename of a scope; nothing was
// renamed then, whatever the reason.
func scopeRenameRefusal(s *session, err error, place api.LabelPlace, scope string) *Error {
	e := labelRefusal(s, err)
	switch {
	case e.APICode == apiCodeLabelScopeMissing || e.APICode == apiCodeLabelScopeMalformed:
		e.Hint = `A scope is the part before "::" of a label name: letters, digits, spaces, "-", "+" and "*", at most 50 characters.`
	case e.APICode == apiCodeLabelNameTaken:
		e.Hint = "A name the rename would give exists already somewhere in the project, so nothing was renamed. List the labels with: minka labels list --project <projectId>"
	case e.Code == CodeNotFound:
		e.Hint = fmt.Sprintf("No label of %s has the scope %q, or the place belongs to a project you are not on the team of. List its labels with: minka labels list %s", placeName(place), scope, placeFlag(place))
	}
	return e
}

func (a *app) newLabelsDeleteCmd() *cobra.Command {
	var placeFlags labelPlaceFlags
	cmd := &cobra.Command{
		Use:   "delete <label>",
		Short: "Delete a label and take it off every item",
		Long: `Deletes a label and takes it off every card, document and upload that carries
it. There is no trash for labels: it is gone. Deleting it again exits 2
(not_found).

` + labelArgHelp + `

` + labelPlaceHelp + `

` + labelWritersHelp,
		Example: `  minka labels delete Blocked --board 0199...
  minka labels delete 0199... --project 0199... --json`,
		Args: requireArgs("label"),
		RunE: func(cmd *cobra.Command, args []string) error {
			place, err := placeFlags.place(cmd)
			if err != nil {
				return err
			}
			s, err := a.session()
			if err != nil {
				return err
			}
			var labels []api.Label
			labelID, label, err := a.resolveLabel(s, place, args[0], "", &labels)
			if err != nil {
				return err
			}
			if err := s.client.DeleteLabel(a.ctx, place, labelID); err != nil {
				return labelRefusal(s, err)
			}
			summary := "Deleted label " + labelID + " from every item"
			if label != nil {
				summary = "Deleted label " + label.Name + " from every item"
			}
			return a.ok(success{
				data:        map[string]any{"id": labelID, "deleted": true},
				summary:     summary,
				breadcrumbs: labelBreadcrumbs(place, ""),
				text:        func(w io.Writer) { fmt.Fprintf(w, "%s.\n", summary) },
			})
		},
	}
	placeFlags.register(cmd)
	return cmd
}

// renderLabels lists labels in the order Minka sent them, with the place each
// belongs to.
func renderLabels(w io.Writer, labels []api.Label) {
	if len(labels) == 0 {
		fmt.Fprintln(w, "No labels.")
		return
	}
	table := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(table, "ID\tNAME\tCOLOR\tOF\tPOSITION")
	for _, label := range labels {
		of := "project"
		if label.ToolID != nil {
			of = "tool"
		}
		fmt.Fprintf(table, "%s\t%s\t%s\t%s\t%d\n", label.ID, label.Name, label.Color, of, label.Position)
	}
	_ = table.Flush()
}

// labelTags are the names of labels in brackets, for one line of a list, and
// nothing where there are none.
func labelTags(labels []api.Label) string {
	if len(labels) == 0 {
		return ""
	}
	var tags []string
	for _, label := range labels {
		tags = append(tags, "["+label.Name+"]")
	}
	return " " + strings.Join(tags, " ")
}

// labelLine is the line of a single item that names its labels.
func labelLine(w io.Writer, labels []api.Label) {
	if len(labels) == 0 {
		return
	}
	var names []string
	for _, label := range labels {
		names = append(names, label.Name)
	}
	fmt.Fprintf(w, "Labels: %s\n", strings.Join(names, ", "))
}

// itemLabelsTarget is what a command that puts labels on or takes them off
// works on: how it finds the item and reads its labels back.
type itemLabelsTarget struct {
	kind    string // the API's segment: cards, documents, files-uploads
	noun    string // card, document, upload
	resolve func(s *session, id *string) error
	read    func(s *session, id string) (labels json.RawMessage, list []api.Label, err error)
	show    string // the command that shows the item
	example string // the command path that labels it, such as "minka docs uploads"
}

func (a *app) cardLabelsTarget() itemLabelsTarget {
	return itemLabelsTarget{kind: "cards", noun: "card", show: "minka cards show", example: "minka cards",
		resolve: func(s *session, id *string) error { return s.resolveCard(a.ctx, id) },
		read: func(s *session, id string) (json.RawMessage, []api.Label, error) {
			card, err := s.client.Card(a.ctx, id)
			if err != nil {
				return nil, nil, err
			}
			return rawField(card.Raw, "labels"), card.Value.Labels, nil
		}}
}

func (a *app) documentLabelsTarget() itemLabelsTarget {
	return itemLabelsTarget{kind: "documents", noun: "document", show: "minka docs show", example: "minka docs",
		read: func(s *session, id string) (json.RawMessage, []api.Label, error) {
			document, err := s.client.Document(a.ctx, id)
			if err != nil {
				return nil, nil, err
			}
			return rawField(document.Raw, "labels"), document.Value.Labels, nil
		}}
}

func (a *app) uploadLabelsTarget() itemLabelsTarget {
	return itemLabelsTarget{kind: "files-uploads", noun: "upload", show: "minka docs uploads show", example: "minka docs uploads",
		read: func(s *session, id string) (json.RawMessage, []api.Label, error) {
			upload, err := s.client.FilesUploadOf(a.ctx, id)
			if err != nil {
				return nil, nil, err
			}
			return rawField(upload.Raw, "labels"), upload.Value.Labels, nil
		}}
}

// newItemLabelCmd is "label" or "unlabel" under the command of an item.
func (a *app) newItemLabelCmd(target itemLabelsTarget, idName string, putOn bool) *cobra.Command {
	article := "a "
	if target.noun == "upload" {
		article = "an "
	}
	use, short := "label", "Put labels on "+article+target.noun
	if !putOn {
		use, short = "unlabel", "Take labels off "+article+target.noun
	}
	long := short + ".\n\n" + labelItemHelp + "\n\n"
	if putOn {
		long += labelScopeHelp + "\n\nA label the " + target.noun + " carries already stays, and that is no refusal."
	} else {
		long += "Taking off a label the " + target.noun + " does not carry is no refusal."
	}
	long += ` A label
Minka does not find exits 9 (validation) with meta.apiCode labelUnknown.
` +
		strings.ToUpper(article[:1]) + article[1:] + target.noun + " in the archive exits 9 with " + target.noun + `Archived,
an archived project with projectArchived.

` + labelWritersHelp
	example := fmt.Sprintf("  %s %s 0199... \"Priority::High\" Blocked\n  %s %s 0199... 0199... --json", target.example, use, target.example, use)
	if target.kind == "cards" {
		example = fmt.Sprintf("  minka cards %s MNK-1A \"Priority::High\" Blocked\n  minka cards %s 0199... 0199... --json", use, use)
	}

	return &cobra.Command{
		Use:     use + " <" + idName + "> <label> [<label>...]",
		Short:   short,
		Long:    long,
		Example: example,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) < 2 {
				return usageErrorf(cmd, "give the %s and at least one label", target.noun)
			}
			return requireID(cmd, idName, &args[0])
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			type wanted struct{ given, id, name string }
			var labels []wanted
			for _, value := range args[1:] {
				if id, isID := asID(value); isID {
					labels = append(labels, wanted{given: value, id: id})
					continue
				}
				if strings.TrimFunc(value, unicode.IsSpace) == "" {
					return usageErrorf(cmd, "a label is named by its ID or its name, and an empty name names none")
				}
				labels = append(labels, wanted{given: value, name: value})
			}

			s, err := a.session()
			if err != nil {
				return err
			}
			itemID := args[0]
			if target.resolve != nil {
				if err := target.resolve(s, &itemID); err != nil {
					return err
				}
			}
			item := api.LabelledItem{Kind: target.kind, ID: itemID}

			var applied []string
			var carried json.RawMessage
			var carriedList []api.Label
			for i, label := range labels {
				var err error
				if putOn {
					var result api.Result[[]api.Label]
					result, err = s.client.AssignLabel(a.ctx, item, label.id, label.name)
					carried, carriedList = result.Raw, result.Value
				} else {
					err = s.client.UnassignLabel(a.ctx, item, label.id, label.name)
				}
				if err != nil {
					e := labelRefusal(s, err)
					if e.Code == CodeNotFound && e.HTTPStatus == http.StatusNotFound {
						draft := ""
						if target.kind == "documents" {
							draft = " is a draft,"
						}
						e.Hint = fmt.Sprintf("The %s does not exist,%s is in the trash, or belongs to a project you are not on the team of.", target.noun, draft)
					}
					notApplied := []string{}
					for _, rest := range labels[i:] {
						notApplied = append(notApplied, rest.given)
					}
					if applied == nil {
						applied = []string{}
					}
					e.Message = fmt.Sprintf("label %q: %s", label.given, e.Message)
					e.Data = map[string]any{"applied": applied, "notApplied": notApplied}
					if len(applied) > 0 {
						e.Notices = append(e.Notices, fmt.Sprintf("applied before the refusal: %s", strings.Join(applied, ", ")))
					}
					return e
				}
				applied = append(applied, label.given)
			}
			if !putOn {
				// The labels are off by now; a failed read takes nothing back,
				// so it is reported with everything applied.
				if carried, carriedList, err = target.read(s, itemID); err != nil {
					e := s.fail(err)
					e.Message = "took the labels off, but could not read them back: " + e.Message
					e.Hint = "Every label was taken off. Read the labels with: " + target.show + " " + itemID
					e.Data = map[string]any{"applied": applied, "notApplied": []string{}}
					return e
				}
			}

			summary := fmt.Sprintf("Put %s on %s %s", plural(len(labels), "label", "labels"), target.noun, itemID)
			if !putOn {
				summary = fmt.Sprintf("Took %s off %s %s", plural(len(labels), "label", "labels"), target.noun, itemID)
			}
			return a.ok(success{
				data:    carried,
				summary: summary,
				breadcrumbs: []Breadcrumb{
					{Action: "show", Cmd: target.show + " " + itemID, Description: "Show the " + target.noun},
				},
				text: func(w io.Writer) {
					fmt.Fprintf(w, "%s.\n", summary)
					if len(carriedList) == 0 {
						fmt.Fprintf(w, "The %s carries no labels.\n", target.noun)
						return
					}
					labelLine(w, carriedList)
				},
			})
		},
	}
}
