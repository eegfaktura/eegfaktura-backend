// Package neverlog is the fixture of TestNeverLog_FindsTheFixture. It is
// compiled by the test only: go build ignores testdata. Lines marked with
// "// want" are hits, all others must stay quiet.
package neverlog

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/sirupsen/logrus"
)

type member struct {
	FirstName string
	IBAN      string
	Number    string
}

type nested struct{ Members []*member }

func run(ctx context.Context, m *member, n nested, err error, id string, count int, when time.Time, format string, list []member, a any) {
	// structs printed whole
	logrus.Infof("member %v", m)                      // want
	logrus.Warnf("member %+v", *m)                    // want
	logrus.WithContext(ctx).Errorf("nested %#v", n)   // want
	logrus.Info(m)                                    // want
	logrus.Debugf("list %s", list)                    // want
	logrus.WithField("MSG", m).Info("x")              // want
	logrus.WithFields(logrus.Fields{"MSG": n}).Warn() // want
	logrus.Infof(format, m)                           // want
	fmt.Printf("%v\n", m)                             // want
	fmt.Println(n)                                    // want

	logrus.Infof("value %+v", a) // want
	logrus.Infof("value %v", a)
	logrus.Errorf("failed: %+v", err)

	// names
	logrus.Warnf("x %s", m.FirstName)                       // want
	logrus.WithContext(ctx).Warn(m.IBAN)                    // want
	logrus.Errorf("x %s", fmt.Sprintf("%s", m.FirstName))   // want
	logrus.WithField("iban", id).Info("x")                  // want
	slog.Info("x", slog.String("email", id))                // want
	slog.WarnContext(ctx, "x", "password", id)              // want
	logrus.Infof("email: %s", id)                           // want
	logrus.Infof("password=%s", id)                         // want
	logrus.WithField("tenant", id).Warnf("token %%v %d", 1) // the verb is escaped, %d is count-like

	// fine
	logrus.Warn("the token expired, no password set")
	logrus.Infof("id %s count %d since %v", id, count, when)
	logrus.Infof("type %T of %p", m, m)
	logrus.WithError(err).Error("failed")
	logrus.Errorf("failed: %v", err)
	logrus.Errorf("failed: %v", errors.New("boom"))
	logrus.WithField("tenant", id).Info(m.Number)
	slog.Info("email sent", slog.String("request.id", id))
}
