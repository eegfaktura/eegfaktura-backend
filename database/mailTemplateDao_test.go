package database

import (
	"context"
	"errors"
	"testing"

	"at.ourproject/vfeeg-backend/model"
	"github.com/pborman/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMailTemplateDao(t *testing.T) {
	ctx := context.Background()
	db, err := GetDB(ctx)
	require.NoError(t, err)
	const tenant, key = "TE000310", "zp-active"
	require.NoError(t, db.DeleteMailTemplate(ctx, tenant, key))

	none, err := db.GetMailTemplate(ctx, tenant, key)
	require.NoError(t, err)
	assert.Nil(t, none)

	img := func() *model.MailTemplateFile {
		return &model.MailTemplateFile{Id: uuid.New(), Tenant: tenant, MailKey: key, Kind: model.MailFileKindImage,
			Filename: "a.png", MimeType: "image/png", Size: 3, Content: []byte("png")}
	}
	ok := func([]model.MailTemplateFile) error { return nil }
	keepImg, dropImg := img(), img()
	require.NoError(t, db.InsertMailTemplateFile(ctx, keepImg, ok))
	require.NoError(t, db.InsertMailTemplateFile(ctx, dropImg, ok))
	pdf := &model.MailTemplateFile{Id: uuid.New(), Tenant: tenant, MailKey: key, Kind: model.MailFileKindAttachment,
		Filename: "a.pdf", MimeType: "application/pdf", Size: 4, Content: []byte("%PDF")}
	require.NoError(t, db.InsertMailTemplateFile(ctx, pdf, ok))

	// the check sees the existing files and can refuse the upload
	refused := errors.New("limit")
	err = db.InsertMailTemplateFile(ctx, img(), func(existing []model.MailTemplateFile) error {
		assert.Len(t, existing, 3)
		return refused
	})
	assert.ErrorIs(t, err, refused)

	saved, err := db.SaveMailTemplate(ctx, &model.MailTemplate{Tenant: tenant, MailKey: key, Subject: "S", BodyHtml: "<p>B</p>",
		DefaultVersion: 1, UpdatedBy: "admin"}, 0, []string{keepImg.Id})
	require.NoError(t, err)
	assert.Equal(t, 1, saved.Version)
	assert.Equal(t, 1, saved.NotifiedDefaultVersion)
	assert.Equal(t, "admin", saved.UpdatedBy)

	files, err := db.ListMailTemplateFiles(ctx, tenant, key, false)
	require.NoError(t, err)
	var ids []string
	for _, f := range files {
		ids = append(ids, f.Id)
		assert.Nil(t, f.Content)
	}
	assert.ElementsMatch(t, []string{keepImg.Id, pdf.Id}, ids, "unreferenced images are deleted, the PDF stays")

	_, err = db.SaveMailTemplate(ctx, &model.MailTemplate{Tenant: tenant, MailKey: key, Subject: "S2", BodyHtml: "<p>B</p>", DefaultVersion: 1, UpdatedBy: "x"}, 0, nil)
	var conflict *MailTemplateVersionError
	require.True(t, errors.As(err, &conflict))
	assert.Equal(t, 1, conflict.Current)

	saved, err = db.SaveMailTemplate(ctx, &model.MailTemplate{Tenant: tenant, MailKey: key, Subject: "S2", BodyHtml: "<p>B</p>", DefaultVersion: 1, UpdatedBy: "x"}, 1, []string{keepImg.Id})
	require.NoError(t, err)
	assert.Equal(t, 2, saved.Version)
	assert.Equal(t, "S2", saved.Subject)

	f, err := db.GetMailTemplateFile(ctx, keepImg.Id)
	require.NoError(t, err)
	assert.Equal(t, []byte("png"), f.Content)
	assert.Equal(t, tenant, f.Tenant)

	deleted, err := db.DeleteMailTemplateFile(ctx, "TE000399", key, keepImg.Id)
	require.NoError(t, err)
	assert.False(t, deleted, "a file is only deleted for its own tenant")

	// default change notice: claimed once
	_, err = testDB.DbInstance.Exec(`UPDATE base.mail_template SET default_version = 0, notified_default_version = 0 WHERE tenant = $1 AND mail_key = $2`, tenant, key)
	require.NoError(t, err)
	claimed, err := db.ClaimMailTemplateDefaultNotices(ctx, key, 1)
	require.NoError(t, err)
	assert.Contains(t, claimed, tenant)
	claimed, err = db.ClaimMailTemplateDefaultNotices(ctx, key, 1)
	require.NoError(t, err)
	assert.NotContains(t, claimed, tenant)

	require.NoError(t, db.DeleteMailTemplate(ctx, tenant, key))
	none, err = db.GetMailTemplate(ctx, tenant, key)
	require.NoError(t, err)
	assert.Nil(t, none)
	files, err = db.ListMailTemplateFiles(ctx, tenant, key, false)
	require.NoError(t, err)
	assert.Empty(t, files, "reset deletes the files too")
}
