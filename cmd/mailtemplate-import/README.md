# mailtemplate-import

One-off operator tool. It moves the hand-made member mail templates from the data volume into the
editable mail texts (`/eeg/mailtemplates`). See konzept-mail-texte-editierbar.md, sections G.4 and C.

## Input

A **copy of the data volume**, not the repository's `public/` folder. It uses the same layout:

```
<base>/<TENANT>/templates/activation-mail-template.toml   (also zp-complete-/zp-inactive-mail-template.toml)
<base>/<TENANT>/templates/<TemplateFile>.html
<base>/<TENANT>/templates/<InlinePictures>  <Attachment>
```

## Conversion

| Go template | new format |
|---|---|
| `{{.Participant.FirstName}}`, `.LastName`, `.ParticipantNumber.String` | `member.firstName`, `member.lastName`, `member.number` |
| `{{.Eeg.Name}}`, `{{.Eeg.Description}}`, `.Contact.Phone.String`, `.Contact.Email.String`, `.Website.String` | `eeg.name`, `eeg.description`, `eeg.phone`, `eeg.email`, `eeg.website` |
| `<ul>{{range .Meteringpoints}}<li>{{.}}</li>{{end}}</ul>` | block `meteringPoints` |
| several consecutive `{{if .Eeg.<field>.Valid}}…{{end}}`, or one around the contact person | block `eeg.contactBlock` |
| a single `{{if .Eeg.Contact.Phone/Email/Website.Valid}}…{{end}}` | condition dropped, inner placeholder kept |
| `<img src="cid:…">` from `InlinePictures` | upload as image, `mailfile:<id>`, size from `max-height`/`max-width` becomes `width` |
| `cid:eegfaktura-logo-1` / `eegfaktura-logo.png` | built-in `mailfile:eegfaktura-logo` (no upload) |
| `Attachment` | upload as PDF |
| `<div>` holding only text (signature lines) | its content followed by `<br>` (the policy unwraps plain divs) |

Any other template expression is a **STOP**. The mail of that tenant is not imported, so no raw
`{{…}}` ever reaches a member. A file that is not PNG/JPEG/PDF, or that is too large, is also a STOP.
Everything the sanitiser removes (fonts, colours, tables, `http:` links, external images) is printed
as a **Hinweis**. Go through these hints with the community.

Subject: the hand-made templates have none. The catalog default (`Aktivierung im Serviceportal`) is used.

## Usage

```bash
go build -o mailtemplate-import ./cmd/mailtemplate-import

# 1. dry run: converted HTML and findings per tenant, writes nothing
./mailtemplate-import -base /copy/of/volume -dry-run

# 2. import (token of a superuser who also has the /EEG_ADMIN group; never as a flag)
export MAILTPL_IMPORT_TOKEN=…
./mailtemplate-import -base /copy/of/volume -api https://<host>/api -tenants RC100181,RC100331 -test-mail eeg
```

| Flag | Meaning |
|---|---|
| `-base` | volume copy (required) |
| `-api` | backend base URL without `/eeg` (required unless `-dry-run`) |
| `-tenants` | comma-separated list; default is every `<TENANT>/templates` under `-base` |
| `-dry-run` | print only |
| `-overwrite` | replace a mail the community has already customised (default: skip it with an error) |
| `-test-mail eeg\|me` | after the import, send a test mail to the community address or to the token's e-mail |

Per mail the tool:
1. reads the current version (`GET`);
2. resets it if `-overwrite` is set;
3. uploads the files;
4. saves with `version: 0`;
5. sends the test mail, if requested.

If a step fails after the first upload, the mail is reset (`DELETE`). The volume template then applies
again. The exit code is 1 if any mail was not imported.

After the community has approved the import (two weeks without objection, G.4), remove the volume template.
