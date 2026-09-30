# Projects

A project carries a **name** and a **description**, and is in use or archived.
`projects list` reads them; `projects edit` renames a project and changes its
description ([C-093](../decisions.md#c-093)). Making a project, archiving it,
putting it back in use, its members and its clients are done on the web.

Everything here was read in Minka's code — `ProjectsApiController`,
`ProjectListItemResource`, `ProjectName`, `ProjectDescription`,
`RenameProjectHandler`, `DbalProjectRepository::rename`,
`ProjectQueryService::isArchivedFor` and `RenameProjectTest`.

## What a read hands over

`GET projects` lists the projects in use the person is a member of, each as
`id`, `name`, `description`, `status` (`active` or `archived`) and `appUrl`;
`?status=archived` lists the archived ones instead. The text view of `projects
list` shows the identifier, the name and the status; the description is in
`--json`, and the text of `projects edit` prints it after a change.

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
refused in the very statement that writes, with the same 404 as somebody who is
not in the project, byte for byte; the hint of the 404 names both.

Minka checks, in this order:

1. The body: 415 `unsupportedMediaType`, 400 `malformedBody` (not JSON, not an
   object, a field that is no string).
2. 422 `nothingToChange`, when neither field is named.
3. The name: `projectNameTooLong`, `projectNameHasControlCharacters`,
   `projectNameMissing`.
4. The description: `projectDescriptionTooLong`.
5. The project: an address that is no identifier is 404; a project the person
   is a **member** of and which is archived is 422 `projectArchived` — a
   client included, who learns it there although a write would be refused
   anyway.
6. A project in use the person is on the team of, else 404.

So everything about the body is refused before the project is looked up, and
somebody outside a project gets a 422 for a description that is too long. The
CLI reaches none of the 400 and 415 cases, and none of an address that is no
identifier: it checks every ID before a request.

A Minka older than this address answers with a 404 page of its web, which is
no JSON, instead of the 404 of its API. `projects edit` tells the two apart by
the body: for such a page it says that the host cannot rename or describe a
project, since the hint of the API's 404 would tell a member they are none.
