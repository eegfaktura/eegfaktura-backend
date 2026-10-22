package database

import (
	"context"
	dbsql "database/sql"
	"errors"
	"fmt"

	"at.ourproject/vfeeg-backend/model"
	"github.com/jmoiron/sqlx"
	"github.com/pborman/uuid"
)

// MailTemplateRepository stores the member mails a community customised
// (base.mail_template, base.mail_template_file).
type MailTemplateRepository interface {
	ListMailTemplates(ctx context.Context, tenant string) ([]model.MailTemplate, error)
	GetMailTemplate(ctx context.Context, tenant, key string) (*model.MailTemplate, error)
	SaveMailTemplate(ctx context.Context, t *model.MailTemplate, expectedVersion int, images []string, check func(all []model.MailTemplateFile) error) (*model.MailTemplate, error)
	DeleteMailTemplate(ctx context.Context, tenant, key string) error
	ListMailTemplateFiles(ctx context.Context, tenant, key string, withContent bool) ([]model.MailTemplateFile, error)
	GetMailTemplateFile(ctx context.Context, id string) (*model.MailTemplateFile, error)
	GetCommittedMailTemplateFiles(ctx context.Context, tenant, key string, ids []string) ([]model.MailTemplateFile, error)
	InsertMailTemplateFile(ctx context.Context, f *model.MailTemplateFile, check func(existing []model.MailTemplateFile) error) error
	DeleteMailTemplateFile(ctx context.Context, tenant, key, id string) (bool, error)
	ClaimMailTemplateDefaultNotices(ctx context.Context, key string, defaultVersion int) ([]string, error)
}

// MailTemplateVersionError is returned when a mail template was changed since
// the caller loaded it (optimistic locking via version).
type MailTemplateVersionError struct {
	Current int
}

func (e *MailTemplateVersionError) Error() string {
	return fmt.Sprintf("mail template was changed, current version %d", e.Current)
}

const mailTemplateColumns = `tenant, mail_key, subject, body_html, version, default_version, notified_default_version, updated_at, updated_by, attachment_file_id`
const mailTemplateFileColumns = `id, tenant, mail_key, kind, filename, mime_type, size, created_at, committed`

// draftMaxAge: older drafts are not listed and are removed by the next save.
const draftMaxAge = `interval '24 hours'`

// notStale selects the files of the saved version and the drafts of the last 24 h.
const notStale = `(committed OR created_at > now() - ` + draftMaxAge + `)`

func (db *sqlDatabase) ListMailTemplates(ctx context.Context, tenant string) ([]model.MailTemplate, error) {
	rows := []model.MailTemplate{}
	err := db.db.SelectContext(ctx, &rows,
		`SELECT `+mailTemplateColumns+` FROM base.mail_template WHERE tenant = $1 ORDER BY mail_key`, tenant)
	return rows, err
}

// GetMailTemplate returns the customised mail of a community, nil if there is none.
func (db *sqlDatabase) GetMailTemplate(ctx context.Context, tenant, key string) (*model.MailTemplate, error) {
	return getMailTemplate(ctx, db.db, tenant, key)
}

func getMailTemplate(ctx context.Context, q sqlx.QueryerContext, tenant, key string) (*model.MailTemplate, error) {
	var t model.MailTemplate
	err := sqlx.GetContext(ctx, q, &t,
		`SELECT `+mailTemplateColumns+` FROM base.mail_template WHERE tenant = $1 AND mail_key = $2`, tenant, key)
	if errors.Is(err, dbsql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// SaveMailTemplate inserts (expectedVersion 0) or updates the customised mail.
// The referenced images and t.AttachmentFileId become part of the version
// (committed); committed files it no longer uses and drafts older than 24 h are
// deleted, fresh drafts of other editors stay. check sees all files of the mail
// and can refuse the save (missing file, limits). A version mismatch is a
// *MailTemplateVersionError.
func (db *sqlDatabase) SaveMailTemplate(ctx context.Context, t *model.MailTemplate, expectedVersion int, images []string,
	check func(all []model.MailTemplateFile) error) (*model.MailTemplate, error) {
	tx, err := db.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, mailTemplateLock(t.Tenant, t.MailKey)); err != nil {
		return nil, err
	}

	current := 0
	err = tx.GetContext(ctx, &current,
		`SELECT version FROM base.mail_template WHERE tenant = $1 AND mail_key = $2 FOR UPDATE`, t.Tenant, t.MailKey)
	if err != nil && !errors.Is(err, dbsql.ErrNoRows) {
		return nil, err
	}
	if current != expectedVersion {
		return nil, &MailTemplateVersionError{Current: current}
	}

	all, err := listMailTemplateFiles(ctx, tx, t.Tenant, t.MailKey, false, true)
	if err != nil {
		return nil, err
	}
	if err = check(all); err != nil {
		return nil, err
	}

	if current == 0 {
		_, err = tx.ExecContext(ctx,
			`INSERT INTO base.mail_template (tenant, mail_key, subject, body_html, version, default_version, notified_default_version, updated_at, updated_by, attachment_file_id)
			 VALUES ($1, $2, $3, $4, 1, $5, $5, now(), $6, $7)`,
			t.Tenant, t.MailKey, t.Subject, t.BodyHtml, t.DefaultVersion, t.UpdatedBy, t.AttachmentFileId)
	} else {
		_, err = tx.ExecContext(ctx,
			`UPDATE base.mail_template SET subject = $3, body_html = $4, version = version + 1, default_version = $5,
			 notified_default_version = GREATEST(notified_default_version, $5), updated_at = now(), updated_by = $6,
			 attachment_file_id = $7
			 WHERE tenant = $1 AND mail_key = $2`,
			t.Tenant, t.MailKey, t.Subject, t.BodyHtml, t.DefaultVersion, t.UpdatedBy, t.AttachmentFileId)
	}
	if err != nil {
		return nil, err
	}

	used := map[string]bool{}
	for _, id := range images {
		used[id] = true
	}
	if t.AttachmentFileId != nil {
		used[*t.AttachmentFileId] = true
	}
	for _, f := range all {
		if used[f.Id] {
			if !f.Committed {
				if _, err = tx.ExecContext(ctx, `UPDATE base.mail_template_file SET committed = true WHERE id = $1`, f.Id); err != nil {
					return nil, err
				}
			}
		} else if f.Committed {
			if _, err = tx.ExecContext(ctx, `DELETE FROM base.mail_template_file WHERE id = $1`, f.Id); err != nil {
				return nil, err
			}
		}
	}
	if _, err = tx.ExecContext(ctx,
		`DELETE FROM base.mail_template_file WHERE tenant = $1 AND mail_key = $2 AND NOT committed AND created_at < now() - `+draftMaxAge,
		t.Tenant, t.MailKey); err != nil {
		return nil, err
	}

	saved, err := getMailTemplate(ctx, tx, t.Tenant, t.MailKey)
	if err != nil {
		return nil, err
	}
	return saved, tx.Commit()
}

func mailTemplateLock(tenant, key string) string {
	return "mail_template_file/" + tenant + "/" + key
}

// DeleteMailTemplate removes the customised mail and all its files.
func (db *sqlDatabase) DeleteMailTemplate(ctx context.Context, tenant, key string) error {
	tx, err := db.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, `DELETE FROM base.mail_template_file WHERE tenant = $1 AND mail_key = $2`, tenant, key); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM base.mail_template WHERE tenant = $1 AND mail_key = $2`, tenant, key); err != nil {
		return err
	}
	return tx.Commit()
}

// ListMailTemplateFiles returns the files of the saved version and the drafts of the last 24 h.
func (db *sqlDatabase) ListMailTemplateFiles(ctx context.Context, tenant, key string, withContent bool) ([]model.MailTemplateFile, error) {
	return listMailTemplateFiles(ctx, db.db, tenant, key, withContent, false)
}

func listMailTemplateFiles(ctx context.Context, q sqlx.QueryerContext, tenant, key string, withContent, includeStale bool) ([]model.MailTemplateFile, error) {
	cols := mailTemplateFileColumns
	if withContent {
		cols += ", content"
	}
	where := `tenant = $1 AND mail_key = $2`
	if !includeStale {
		where += ` AND ` + notStale
	}
	files := []model.MailTemplateFile{}
	err := sqlx.SelectContext(ctx, q, &files,
		`SELECT `+cols+` FROM base.mail_template_file WHERE `+where+` ORDER BY created_at, id`, tenant, key)
	return files, err
}

// GetMailTemplateFile loads a file by id only; the caller asserts tenant and mail key.
func (db *sqlDatabase) GetMailTemplateFile(ctx context.Context, id string) (*model.MailTemplateFile, error) {
	var f model.MailTemplateFile
	err := db.db.GetContext(ctx, &f,
		`SELECT `+mailTemplateFileColumns+`, content FROM base.mail_template_file WHERE id = $1`, id)
	if errors.Is(err, dbsql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &f, nil
}

// GetCommittedMailTemplateFiles loads, with content, only the given files that
// belong to the saved version of this tenant and mail. Unknown or draft ids are
// left out; the caller decides whether that is an error.
func (db *sqlDatabase) GetCommittedMailTemplateFiles(ctx context.Context, tenant, key string, ids []string) ([]model.MailTemplateFile, error) {
	files := []model.MailTemplateFile{}
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] || uuid.Parse(id) == nil {
			continue
		}
		seen[id] = true
		var f model.MailTemplateFile
		err := db.db.GetContext(ctx, &f,
			`SELECT `+mailTemplateFileColumns+`, content FROM base.mail_template_file
			 WHERE id = $1 AND tenant = $2 AND mail_key = $3 AND committed`, id, tenant, key)
		if errors.Is(err, dbsql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, err
		}
		files = append(files, f)
	}
	return files, nil
}

// InsertMailTemplateFile stores an upload as a draft after check accepted the
// files the mail already has. Uploads and saves of the same mail are
// serialised, so the limits hold.
func (db *sqlDatabase) InsertMailTemplateFile(ctx context.Context, f *model.MailTemplateFile, check func(existing []model.MailTemplateFile) error) error {
	tx, err := db.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, mailTemplateLock(f.Tenant, f.MailKey)); err != nil {
		return err
	}
	existing, err := listMailTemplateFiles(ctx, tx, f.Tenant, f.MailKey, false, false)
	if err != nil {
		return err
	}
	if err = check(existing); err != nil {
		return err
	}
	if err = tx.GetContext(ctx, &f.CreatedAt,
		`INSERT INTO base.mail_template_file (id, tenant, mail_key, kind, filename, mime_type, size, content, committed)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, false) RETURNING created_at`,
		f.Id, f.Tenant, f.MailKey, f.Kind, f.Filename, f.MimeType, f.Size, f.Content); err != nil {
		return err
	}
	return tx.Commit()
}

// DeleteMailTemplateFile deletes a file of a community's mail; false if there was none.
func (db *sqlDatabase) DeleteMailTemplateFile(ctx context.Context, tenant, key, id string) (bool, error) {
	res, err := db.db.ExecContext(ctx,
		`DELETE FROM base.mail_template_file WHERE id = $1 AND tenant = $2 AND mail_key = $3`, id, tenant, key)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// ClaimMailTemplateDefaultNotices marks the customised mails whose default
// changed since they were saved as notified and returns their tenants. Each
// row is returned once, also when several replicas start at the same time.
func (db *sqlDatabase) ClaimMailTemplateDefaultNotices(ctx context.Context, key string, defaultVersion int) ([]string, error) {
	tenants := []string{}
	err := db.db.SelectContext(ctx, &tenants,
		`UPDATE base.mail_template SET notified_default_version = $2
		 WHERE mail_key = $1 AND default_version < $2 AND notified_default_version < $2
		 RETURNING tenant`, key, defaultVersion)
	return tenants, err
}
