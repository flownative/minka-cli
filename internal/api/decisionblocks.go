package api

// DecisionBlock is one decisions block of a document's or a post's body as
// its reader may see it: the query the body stores and the decisions it
// lists now. Position counts the minka-decisions elements of the body from 0,
// so each block's rows belong where its element stands.
type DecisionBlock struct {
	Position    int                     `json:"position"`
	ToolID      string                  `json:"toolId"`
	LabelID     *string                 `json:"labelId"`
	StatusID    *string                 `json:"statusId"`
	DecidedInID *string                 `json:"decidedInId"` // the event the decisions were taken in
	AppliesToID *string                 `json:"appliesToId"` // the event or card they apply to
	Tool        *DecisionsToolReference `json:"tool"`        // nil where the reader sees no such tool in the project
	Summary     string                  `json:"summary"`     // the tool and every filter by name, in English
	// Frozen says the rows are the copies made when the document was
	// locked; each may then say what changed since in Today.
	Frozen    bool                 `json:"frozen"`
	Decisions []DecisionBlockEntry `json:"decisions"`
	// Markdown is the same as a list whose words are escaped, so that none
	// becomes markup.
	Markdown string `json:"markdown"`
}

// DecisionBlockEntry is a decision as a decisions block lists it.
type DecisionBlockEntry struct {
	ID           string              `json:"id"`
	ShortID      *string             `json:"shortId"` // nil for a decision its tool has not numbered
	Title        string              `json:"title"`
	Status       DecisionStatus      `json:"status"` // in a frozen block the status at the lock
	VotesFor     *int                `json:"votesFor"`
	VotesAgainst *int                `json:"votesAgainst"`
	Abstentions  *int                `json:"abstentions"`
	VotesNote    *string             `json:"votesNote"`
	Today        *DecisionBlockToday `json:"today"` // only in a frozen block
	AppURL       string              `json:"appUrl"`
}

// DecisionBlockToday is what changed in a decision since a locked document
// copied it.
type DecisionBlockToday struct {
	Status       *DecisionStatus     `json:"status"`       // nil while the status is the one copied
	SupersededBy *DecisionBlockLater `json:"supersededBy"` // the newest visible decision superseding it
	Reworded     bool                `json:"reworded"`
}

// DecisionBlockLater is the decision that superseded one a frozen block
// copied.
type DecisionBlockLater struct {
	ID      string  `json:"id"`
	ShortID *string `json:"shortId"`
	Title   string  `json:"title"`
	AppURL  string  `json:"appUrl"`
}
