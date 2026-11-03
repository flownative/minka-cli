# Steps

A **step** is one line of work on a card: a text of at most 500 characters,
ticked off or open, with the **people** on it and the **day it is due**. It is
no item of its own: it has no comments and no address but under its card,
and nobody is told when put on one. So its commands live under `cards`, beside
`cards notes`, and nowhere else ([C-125](../decisions.md#c-125)).
`cards steps list` reads the steps of a card, `cards steps create` adds one,
`cards steps edit` changes its text, its people and its day
([C-100](../decisions.md#c-100)), `cards steps move` puts it elsewhere in the
list ([C-123](../decisions.md#c-123)), `cards steps complete` and
`cards steps reopen` tick it off and open it again, and `cards steps delete`
takes it off.

Everything here was read in Minka's code — `BoardApiController::addStepAction`,
`updateStepAction`, `moveStepAction` and `removeStepAction`, `StepResource`,
`StepTitle`, `EditStepHandler`, `RenameStepHandler`, `MoveStepHandler`, `Gap`,
`DbalCardStepRepository` — and in `ApiStepAssigneesTest`, `ApiCardStepsTest`,
`ApiArchivedCardTest` and `ApiArchivedProjectTest`.

## What a read hands over

A card read by itself carries `steps` in the order it shows them, each as
`id`, `title`, `completed`, `assignees`, `dueOn` and `card`, which names the
card a step is and is null for a step in words (see
[A card as a step](#a-card-as-a-step)). `assignees` are
references in the order they were put on, and somebody who left the project
stays on a step and is named there until taken off. `dueOn` is `YYYY-MM-DD` or
null; a step says nothing about being overdue. A board carries only
`stepCount` and `completedStepCount` per card.

The text view of `cards steps list` and `cards show` puts the people and the day
behind the line: `2. [ ] Call the caterer (Anke, due 2026-10-09)`.

A step is named by its ID, by its position from 1 or by its text, which needs
the card, so every command but `create` reads the card first. An ID that is on
no step goes out as it is, and Minka gives its answer.

## Writing a step

`POST cards/{cardId}/steps` adds a step **at the end** from `title` alone and
answers the card (201). People and a day come with `cards steps edit` afterwards.

`PATCH cards/{cardId}/steps/{stepId}` changes any of `title`, `completed`,
`dueOn` and `assigneeIds` in one request, tested by key: what the body leaves
out stays, and `"assigneeIds": null` counts as left out. A body that names none
of the four is `nothingToChange`. Asking for the state a step is in already
succeeds, and a change of the people and the day to what the step carries
already writes nothing and succeeds.

**The text.** `title` follows the rules of a new step: trimmed at both ends,
at most 500 characters, no control characters, line breaks and tabs included,
and not empty; `null` counts as empty. `cards steps edit --title` sends what it is
given, `""` too, so Minka refuses it with `stepMissing`. Unlike the text of
`cards steps create`, `--title -` reads nothing from stdin, as no `--title` of the
CLI does. The same text again is written again and succeeds.

**The day.** `dueOn` is a day as `YYYY-MM-DD`; null and `""` take it off. So
`cards steps edit --due ""` sends null.

**The people.** `assigneeIds` is **the whole set** the step is to carry: whoever
stays keeps their place, whoever is new comes behind in the order named, and
whoever is not named again is taken off. So `cards steps edit --assignee` names the
whole set, and `--no-assignees` sends an empty list — never null, which Minka
reads as no change. A person is an ID, `me`, an e-mail address or a name; the
members of the card's project are read only where a name, an address or `me`
asks for them. Only a **member of the project who sees the board** can be put
on a step, a client included where the board is shown to clients: anybody else
refuses the whole write with `personNotAProjectMember`, and neither the people
nor the day are written. Somebody who left the project and is on the step
already may be named again without a refusal.

The day and the people are one write, the text a second one behind it, and a
tick named beside them a third. So a person refused leaves the text as it
was. `cards steps edit` never sends the tick.

**An older Minka.** A Minka from before the text of a step could change does
not know `title`: it answers `--title` alone with `nothingToChange`, and
beside other flags writes them and passes over the text with a 200. The CLI
compares the text of the answer with the one sent as Minka keeps it — invalid
UTF-8 replaced by U+FFFD as Go's JSON encoder sends it, then trimmed of PHP's
blanks only, so a no-break space stays — and exits 7 (`api_error`) with the
card in `data` where they differ. The message says the rest was written only
where a day or people were sent along; both answers carry a hint that says so.

## Moving a step

`POST cards/{cardId}/steps/{stepId}/moves` puts the step between
`previousStepId` and `nextStepId` and answers the card (200), also where it
stands there already. A neighbour given as `null` or `""` is none, and with
neither the step goes to the top. Where both are given, the previous one
decides and the next one is only checked: the step goes right behind the
previous step, whatever the next one says. The end has no word of its own:
the last of the other steps is the previous one.

`cards steps move` takes exactly one of `--top`, `--bottom`, `--after <step>` and
`--before <step>`, the neighbour named like the step. `--top` sends neither
neighbour, `--after` the previous one, `--before` the next one. `--bottom`
means behind the last step **as read**: it sends the last step of the card
that is not the one moved as the previous one, read from the card the command
reads first anyway, and neither where the card has no other step. Where that
step leaves the card before the move, Minka answers `stepPlaceUnknown`, and
the hint says that the last step is gone, not that `--after` or `--before`
named a wrong one. The step named as its own neighbour goes out, and
Minka answers `stepPlaceUnknown`.

A Minka from before the move has no address for it and answers 404. Only where
the step was on the card just read does the hint say that this may be the
reason; an ID on no step gets the hint of any 404.

An edit with no flag, or with both `--assignee` and `--no-assignees`, is a
usage error (exit 1) before any request.

## A card as a step

A step is either words or **another card**. A card step has no title, tick,
people or day of its own: it reads as its card stands for the reader. Its
`title` is the card's, `completed` says the card is in Done, `assignees` is
always empty and `dueOn` null, and `card` is the card as `{kind: "card", id,
shortId, title, projectId, boardId, column, columnColor, assignees, appUrl}`,
without `shortUrl` and the people left out for a card in the archive. A step
in words carries `card: null`. A card the reader may not see is
`{"kind": "card"}` alone, and its step has the title `""`. A card read by
itself carries `stepOf`, the cards it is a step of that the reader sees, in
the same shape and never as `{kind: "card"}`; a board's tiles do not. A
board's `completedStepCount` counts the ticked steps in words and the card
steps whose card the reader sees in Done.

`cards steps create <card> --card <child>` sends `{"cardId": …}` in place of
`title`: the text and `--card` exclude each other, and a `--card` given empty
or blank names no card; both are usage errors before any request. The child
goes out as Minka reads a card's name
([C-153](../decisions.md#c-153)): one leading `#` comes off, an address of
this host — the short one `…/go/card/<shortId>` or the long one ending in
`…/cards/<id>`, query and fragment ignored — gives up the name in it, and an
ID goes out in lowercase. Anything else, an address of another host included,
goes out as given, and Minka answers `stepCardUnknown`. The card can be on any
board of any project the reader sees, in the archive too, and a step of several
cards; it is a step of the same card at most once. Minka answers 201 with the
card and the new step at the end, and 200 with the card unchanged where the
child is a step of it already; the summary says which, and `data` is the card
as answered.

`cards steps make-card <card> <step>` ([C-154](../decisions.md#c-154)) posts
to `cards/{cardId}/steps/{stepId}/card` without a body. Minka adds a card in
Triage of the same board with the words of the step as its title and the
step's people and day, and answers 201 with the new card, whose `stepOf` names
the card the step is on, and its address in `Location`. The step keeps its ID
and place and becomes a card step; its tick and people are gone with it.
Words longer than 200 characters give the title in 199 graphemes, trimmed at
the end, with `…`, and the whole words as the description `<p>…</p>`. `data`
is the new card; the text view names it, its column and the position of the
step that now is that card.

A card step takes no `PATCH`: `stepIsACard`, after the three 404s and before
the body is read or the archive is told. So `cards steps edit`, `complete`
and `reopen` on a card step exit 9, and the hint names the card to change,
with `minka cards move <child> --column Done` to finish it; for a card the
reader cannot see, the hint offers only to remove the link with `cards steps
delete`. A card step moves as any step and is a neighbour as any. `cards steps
delete` removes the link alone, and the summary says the card itself stays.
The summaries name a step in words by its text, `step "Call the caterer"`, a
card step by its card, `card step #MNK-12 Buy pegs` (the title alone where
there is no short identifier), and one whose card the reader cannot see by its
position, `step 2, a card you cannot open`.

The text views ([C-155](../decisions.md#c-155)) show a card step as its card,
without a tick box: `2. ↳ #MNK-12 Buy pegs  [Doing] (Dora) <address>`, the
address the short one built on the host of `appUrl` where the card has a short
identifier, and `appUrl` where not; Done reads as done by its column. A card the
reader cannot see is `2. ↳ (a card you cannot open)`. `cards show` names the
parents in the line `Step of: #MNK-1 Title, …` right below the title. A step in
words reads as it always did, and a Minka without card steps, whose steps carry
no `card` and whose cards no `stepOf`, reads as before.

A step is never named by an empty text: the step of a card the reader cannot
see is named by its position or its ID only.

**An older Minka.** A Minka without card steps passes over `cardId` and
refuses the missing text with `stepMissing`; `make-card` gets the 404 page of
an address it does not have. Both exit 2 (`not_found`) with a message that the
host does not know a card as the step of another, as `cards next` does against
a Minka without its address (C-149).

## The order of the refusals

Minka checks in this order, and each step comes only where the one before it
passed:

1. The card: 404 for one that is unknown, foreign, hidden from a client or in
   the trash. Then the step: 404 for an identifier that is none and for a step
   that is not on this card, another card's included.
2. The body: 415 `unsupportedMediaType`, 400 `malformedBody` for a body that is
   no JSON object and for `assigneeIds` that is no list of strings — read
   before anything else of the body.
3. 422 `nothingToChange`, then `stepCompletedMissing` for a `completed` that is
   no `true` or `false`, then 400 for a `title` that is no string, then
   `stepTooLong`, `stepHasControlCharacters` and `stepMissing`, then 400 for a
   `dueOn` that is no string, then `stepDueDateNotADate`, then
   `personNotAProjectMember` for an identifier of a person that is none.
4. Where the day or the people are named, the write: `projectArchived`, then
   `cardArchived`, then `personNotAProjectMember` for somebody who may not be
   on the step. An archived project or card refuses even a change to what the
   step carries already.
5. Where the text is named: `projectArchived`, then `cardArchived`.
6. Where the tick is named: `projectArchived`, then `cardArchived`.

So an identifier of a person that is none and a text Minka would refuse are
refused before the archive is told, and a person who may not be on the step
only after it.

A move is checked in this order:

1. The card and the step, as above: 404 for a step that is not on this card,
   before the body is read.
2. The body: 415, 400 `malformedBody`.
3. `previousStepId`, then `nextStepId`: 400 for one that is no string; for one
   that is no identifier, the step itself or no other step of this card,
   `projectArchived`, then `cardArchived`, then `stepPlaceUnknown`. A wrong
   previous neighbour is refused before the next one is read.
4. The move: `projectArchived`, then `cardArchived`, even for a step that
   stands where it is asked to go. The CLI reaches
none of the 400 and 415 cases and no identifier that is none: it checks every
ID before a request.

A card as a step is checked in this order:

1. The card: 404, an archived one found as well. Then the body: 415, 400.
2. `title` beside `cardId`, whatever its value: `stepTitleAndCard`. Then 400
   for a `cardId` that is no string.
3. The name, trimmed: an ID in any case or a short identifier in any case;
   anything else, null and `""` included, and a card the person may not see,
   in the trash, of another organization or named ambiguously, is
   `stepCardUnknown`. The card itself is `stepCardIsThisCard`, before the
   archive is told.
4. A child that is a step already: `projectArchived`, `cardArchived`, else
   200. Then the write: `projectArchived`, `cardArchived`, else 201.

Making a step into a card reads no body and is checked in this order: the
card 404, a step ID that is none 404, `projectArchived` and `cardArchived`,
a step not on the card 404, a card step `stepIsACard`, the words as the
title of a card (`cardTitleHasInvisibleCharacters`, `cardTitleMissing`,
`cardTitleTooLong`, `cardTitleHasControlCharacters`), then the people
(`personNotAProjectMember`, `agentOfAnotherPerson`). A refusal writes
nothing.

| Answer | Command | Exit |
|---|---|---|
| 404 | every command, for a card that is unknown, foreign or in the trash; `cards steps edit`, `move`, `complete` and `reopen` for a step that is not on the card; `cards steps move` against a Minka without the move | 2 |
| 422 `stepMissing`, `stepTooLong`, `stepHasControlCharacters` | `cards steps create`, `cards steps edit --title` | 9 |
| 422 `stepPlaceUnknown` | `cards steps move` with the step itself as its neighbour; the hint names `minka cards steps list` | 9 |
| 422 `stepDueDateNotADate` | `cards steps edit` | 9 |
| 422 `personNotAProjectMember` | `cards steps edit`; the hint names `minka people list` | 9 |
| 422 `projectArchived`, `cardArchived` | every write; for `cardArchived` the hint names `cards unarchive` | 9 |
| 422 `stepCompletedMissing` | `cards steps edit` against a Minka older than the people and the day of a step, which reads every change as a tick; the hint says so | 9 |
| 422 `nothingToChange` | `cards steps edit --title` alone against a Minka without the text of a step, which the hint says; otherwise unreachable: an edit without a change is a usage error here | 9 |
| 200 without the text sent | `cards steps edit --title` beside other flags against a Minka without the text of a step | 7 |
| 422 `stepCardUnknown` | `cards steps create --card`; the hint names the forms a card is named by | 9 |
| 422 `stepCardIsThisCard` | `cards steps create --card` with the card itself | 9 |
| 422 `stepTitleAndCard` | unreachable: the CLI sends the text or the card; the hint calls it a fault in minka | 9 |
| 422 `stepIsACard` | `cards steps edit`, `complete` and `reopen` on a card step, the hint naming the card to change; `cards steps make-card` on a card step | 9 |
| 422 `cardTitle…`, `personNotAProjectMember`, `agentOfAnotherPerson` | `cards steps make-card` | 9 |
| 422 `stepMissing` for `--card`, 404 page | `cards steps create --card` and `cards steps make-card` against a Minka without card steps | 2 |

`cards steps delete` answers 204 for a step that is not on the card as well, and is
refused only for an archived project or card.
