# Steps

A **step** is one line of work on a card: a text of at most 500 characters,
ticked off or open, with the **people** on it and the **day it is due**. It is
no item of its own: it has no comments and no address but under its card,
and nobody is told when put on one. `steps list` reads the steps of a card, `steps create` adds
one, `steps edit` changes its people and its day
([C-100](../decisions.md#c-100)), `steps complete` and `steps reopen` tick it
off and open it again, and `steps delete` takes it off. Renaming a step and
putting it elsewhere in the list have no address in the API.

Everything here was read in Minka's code — `BoardApiController::addStepAction`,
`updateStepAction` and `removeStepAction`, `StepResource`, `EditStepHandler`,
`DbalCardStepRepository` — and in `ApiStepAssigneesTest` and
`ApiCardStepsTest`.

## What a read hands over

A card read by itself carries `steps` in the order it shows them, each as
`id`, `title`, `completed`, `assignees` and `dueOn`. `assignees` are
references in the order they were put on, and somebody who left the project
stays on a step and is named there until taken off. `dueOn` is `YYYY-MM-DD` or
null; a step says nothing about being overdue. A board carries only
`stepCount` and `completedStepCount` per card.

The text view of `steps list` and `cards show` puts the people and the day
behind the line: `2. [ ] Call the caterer (Anke, due 2026-10-09)`.

A step is named by its ID, by its position from 1 or by its text, which needs
the card, so every command but `create` reads the card first. An ID that is on
no step goes out as it is, and Minka gives its answer.

## Writing a step

`POST cards/{cardId}/steps` adds a step **at the end** from `title` alone and
answers the card (201). People and a day come with `steps edit` afterwards.

`PATCH cards/{cardId}/steps/{stepId}` changes any of `completed`, `dueOn` and
`assigneeIds` in one request, tested by key: what the body leaves out stays,
and `"assigneeIds": null` counts as left out. A body that names none of the
three is `nothingToChange`. Asking for the state a step is in already
succeeds, and a change of the people and the day to what the step carries
already writes nothing and succeeds.

**The day.** `dueOn` is a day as `YYYY-MM-DD`; null and `""` take it off. So
`steps edit --due ""` sends null.

**The people.** `assigneeIds` is **the whole set** the step is to carry: whoever
stays keeps their place, whoever is new comes behind in the order named, and
whoever is not named again is taken off. So `steps edit --assignee` names the
whole set, and `--no-assignees` sends an empty list — never null, which Minka
reads as no change. A person is an ID, `me`, an e-mail address or a name; the
members of the card's project are read only where a name, an address or `me`
asks for them. Only a **member of the project who sees the board** can be put
on a step, a client included where the board is shown to clients: anybody else
refuses the whole write with `personNotAProjectMember`, and neither the people
nor the day are written. Somebody who left the project and is on the step
already may be named again without a refusal.

The day and the people are one write, and a tick named beside them a second
one behind it. `steps edit` never sends the tick.

An edit with no flag, or with both `--assignee` and `--no-assignees`, is a
usage error (exit 1) before any request.

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
   no `true` or `false`, then 400 for a `dueOn` that is no string, then
   `stepDueDateNotADate`, then `personNotAProjectMember` for an identifier of a
   person that is none.
4. Where the day or the people are named, the write: `projectArchived`, then
   `cardArchived`, then `personNotAProjectMember` for somebody who may not be
   on the step. An archived project or card refuses even a change to what the
   step carries already.
5. Where the tick is named: `projectArchived`, then `cardArchived`.

So an identifier of a person that is none is refused before the archive is
told, and a person who may not be on the step only after it. The CLI reaches
none of the 400 and 415 cases and no identifier that is none: it checks every
ID before a request.

| Answer | Command | Exit |
|---|---|---|
| 404 | every command, for a card that is unknown, foreign or in the trash; `steps edit`, `complete` and `reopen` for a step that is not on the card | 2 |
| 422 `stepMissing`, `stepTooLong`, `stepHasControlCharacters` | `steps create` | 9 |
| 422 `stepDueDateNotADate` | `steps edit` | 9 |
| 422 `personNotAProjectMember` | `steps edit`; the hint names `minka people list` | 9 |
| 422 `projectArchived`, `cardArchived` | every write; for `cardArchived` the hint names `cards unarchive` | 9 |
| 422 `stepCompletedMissing` | `steps edit` against a Minka older than the people and the day of a step, which reads every change as a tick; the hint says so | 9 |
| 422 `nothingToChange` | unreachable: an edit without a change is a usage error here | — |

`steps delete` answers 204 for a step that is not on the card as well, and is
refused only for an archived project or card.
