# Projects

A project carries a **name**, a **description** and a **category**, and is in
use or archived. `projects list` reads them; `projects create` starts a
project, `projects edit` renames it and changes its description
([C-093](../decisions.md#c-093)). `people add` and `people remove` keep its
members, `tools …` its tools of every type. Archiving a project, putting it
back in use, letting clients in and bringing a tool back from the trash are
done on the web.

Everything here was read in Minka's code — `ProjectsApiController`,
`ProjectListItemResource`, `ProjectName`, `ProjectDescription`,
`RenameProjectHandler`, `DbalProjectRepository::rename`,
`ProjectQueryService::isArchivedFor` and `RenameProjectTest`.

## What a read hands over

`GET projects` lists the projects in use the person is a member of, each as
`id`, `name`, `description`, `status` (`active` or `archived`), `appUrl` and
`category`; `?status=archived` lists the archived ones instead. The text view
of `projects list` shows the identifier, the name and the status, and a column
CATEGORY where at least one project is filed under a category. The
description is in `--json`, and the text of `projects edit` prints it after a
change.

## What an edit sends

`PATCH projects/{projectId}` takes `name`, `description` or both. The CLI sends
only the flags given: a field left out — and one sent as `null` — stays as it
stands, and Minka writes only what is named, so a rename does not undo a
description somebody else wrote meanwhile. There is therefore no `--expect`:
the one write that could overwrite another's is the very field one names.

The **description is plain text**, a sentence or two on what the project is
about. Minka stores it as sent, trimmed, and shows it as text: no Markdown is
converted, no HTML is kept as markup, nothing becomes a mention or a
reference, and no guard over files applies. `-` reads it from stdin, and
`--description ""` empties it.

The **name** is one line: trimmed, at most 200 characters, no control
character. An empty name is refused, never read as "leave it".

An edit with neither flag is a usage error (exit 1) before any request: Minka
would answer `nothingToChange`.

The answer is the project as `GET projects` lists it.

## Who may, and the order of the refusals

Only the **team** of a project renames it. A **client** of the project is
refused with the same 404 as somebody who is not in the project, byte for
byte; the hint of the 404 names both.

Minka checks, in this order:

1. The project: an address that is no identifier, a project the person is not
   a member of and a project the person is a **client** of are 404.
2. The body: 415 `unsupportedMediaType`, 400 `malformedBody` (not JSON, not an
   object, a field that is no string).
3. 422 `nothingToChange`, when neither field is named.
4. The name: `projectNameTooLong`, `projectNameHasControlCharacters`,
   `projectNameMissing`.
5. The description: `projectDescriptionTooLong`.
6. An archived project is 422 `projectArchived`.

So somebody outside a project, and a client of it, learn nothing but the 404,
whatever the body says. The CLI reaches none of the 400 and 415 cases, and
none of an address that is no identifier: it checks every ID before a request.

A Minka older than this address answers with a 404 page of its web, which is
no JSON, instead of the 404 of its API. `projects edit` tells the two apart by
the body: for such a page it says that the host cannot rename or describe a
project, since the hint of the API's 404 would tell a member they are none.

## Starting a project

`projects create` sends `POST projects` with `name`, and `description` and
`category` where their flags are given; `--name` is required, or exit 1
before any request. The name and the description follow the rules of an edit.
The **category** is a name of the organization's list, compared without case
and answered as the list writes it; a name the list does not hold is 422
`projectCategoryUnknown`, and no project is started. The list itself is kept
on the web and has no address in the API; the hint says so, and that
`projects list` shows the categories your projects are filed under. A new
project has no tools and does not work with clients.

The creator is the first member. An **agent** starts the project together
with the person it acts for, who joins it as well. A client of the
organization, and an agent acting for one, are refused with 403
`organizationTeamOnly` (exit 4). Minka checks the name, the description and
the category before it asks who starts the project.

The answer is 201 with the project as `GET projects` lists it, with its
`category`: the name it is filed under, or null. A client of a project is
always told null. The text view of `projects list` shows a column of the
category where at least one project is filed under one.

## Members

`people add <projectId> <personId>` sends `POST projects/{projectId}/people`
with `personId`. The answer is the member as `people list` lists them: 201
where they are new, 200 where they were in already. The summary says which,
naming the person.
Minka checks, in this order:

1. The project: one the person is not in, and one they are a client of, are
   404.
2. The body; no `personId` is 422 `personIdMissing`.
3. A `personId` that is no UUID is 422 `personNotAnOrganizationMember`.
4. Somebody in already is 200, also in an archived project.
5. An ID of nobody, or of somebody outside the organization, is 422
   `personNotAnOrganizationMember`.
6. An archived project is 422 `projectArchived`.
7. `agentRetired` for an agent that left service, `projectWithoutClientAccess`
   for a client of the organization and a project that does not work with
   clients, `principalNotAProjectMember` for an agent whose human is not in
   the project.

`people remove <projectId> <person>` sends `DELETE
projects/{projectId}/people/{personId}`. The person is given as for `cards
assign`: by ID, as `me`, by e-mail address or by name, looked up among the
people of the project; an ID no member has goes to Minka as it is. `me` is how
one leaves a project. The agents acting for the person go with
them. Anybody may leave, an archived project and one they are a client of
included. Minka answers 204, or in this order: 404 for a project the person is
not in and for somebody not in the project; 422 `projectArchived` for taking
out somebody else from an archived project; 404 for a client who names
somebody else; 422 `lastProjectMember` where no other human of the team would
stay. The summary names the person taken out, by name where the people of the
project give one; the project is in `--json` as `projectId`.

## Tools

The tools of a project, of every type, are one resource:
`projects/{projectId}/tools`. A tool carries `id`, `projectId`, `type`,
`title`, `position` and `appUrl`. The **type** is one of `board`, `chat`,
`posts`, `todos`, `checkins`, `files`, `forwards`, `calendar` and
`decisions`. The **position** counts from 1 the tools the reader sees: a tool
out of use or in the trash is not listed, nor is a type only a newer Minka
knows, and a client sees only the tools shown to clients. An archived project
still lists its tools.

Every command on one tool takes `<projectId> <toolId>`, the two IDs of the
address ([C-189](../decisions.md#c-189)); a tool is found among those the
project lists, so one out of use is 404.

- `tools list <projectId>` lists them in the order of the project page: the
  position, the type, the ID and the title.
- `tools add <projectId> --type <type> --title <title>` puts a new tool at the
  end, filled as the web fills it: a board with its system columns, Posts
  with the default kinds. Both flags are required, or exit 1. The type goes
  as typed ([C-187](../decisions.md#c-187)): Minka refuses another, one in
  capitals and `forwards`, which only an import brings, with
  `toolTypeUnknown`, and its message names the types it takes; the CLI keeps
  no list of them. The answer is 201 with the tool as listed.
- `tools rename <projectId> <toolId> --title <title>` renames a tool of any
  type, `forwards` included; without `--title` it is exit 1, where Minka
  would answer `nothingToChange`.
- `tools move <projectId> <toolId> [--after <toolId> | --before <toolId>]`
  sends `previousToolId` or `nextToolId`, and with neither flag `{}`, which
  puts the tool in front ([C-188](../decisions.md#c-188)). Both flags, or the
  tool itself as its neighbour, are exit 1 before any request. A neighbour
  that is no other tool the project lists is 422 `toolPlaceUnknown`. The
  answer is the tool with its new position.
- `tools trash <projectId> <toolId>` puts the tool and everything it holds in
  the trash of the project: 204, a second time 404. `trash list` shows it as
  kind `tool` with its `toolType`, not restorable: only the web brings it
  back. The summary names the tool by its ID, as `todolists trash` does: Minka
  answers the trash with no body, and a read of the tools only for the summary
  is not worth a request.

A title is one line of at most 200 characters: `toolTitleMissing`,
`toolTitleTooLong` and `toolTitleHasControlCharacters`. For every write Minka
checks the project (404 for an outsider and for a client, before anything
else), the tool, the body, the type, the title and the neighbour, and then
whether the project is archived (422 `projectArchived`).

## Refusals

Every refusal named here is exit 9 with `meta.apiCode`, but for
`organizationTeamOnly` (exit 4) and the 404s (exit 2). The hint of a 404 says,
per command, what may not be there: the project, the person, the tool, or
the right of a client ([C-190](../decisions.md#c-190)). A Minka older than
these addresses answers with a 404 page of its web, not with Minka's JSON;
then `tools list` says that the host cannot list the tools of a project yet,
and every other of these commands that it cannot set up projects yet.
Where the code alone does not say what to do, the hint does: who may start a
project, where the categories are kept, that an agent's human has to be added
first, that a project keeps one human on its team. A command such a hint
suggests is written out with the project's ID: `people add <projectId>` after
`lastProjectMember`, `tools list <projectId>` after `toolPlaceUnknown`. The
hints of a 404 keep the `<projectId>` placeholder, because the project may
not exist.

What this section and the three above say was read in
`ProjectsApiController`, `ProjectToolsApiController`, `ProjectToolResource`,
`CreateProjectHandler`, `ProjectJoining`, `RemoveProjectMemberHandler`,
`ToolTypeRegistry` and `ApiProjectSetupTest`.
