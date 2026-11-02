package logging

import (
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/constant"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The never-log test (concept W-261008-ca2, point 4; dev#45 L11). It
// type-checks the module's source and fails on
//   - a struct (a model above all) printed whole: by a format verb, as a bare
//     argument or as a field value. Errors and Stringers print what they choose.
//   - a name of personal data or a secret in the arguments of a log call, in a
//     field key, or right before a format verb in the text.
//
// A log call is logrus, log/slog, log or fmt.Print*. Generated code is left
// out. A line that is fine goes on neverLogAllowed with its reason, after
// reading what the value holds at runtime.

// neverLogNames match case-insensitively inside identifiers, field keys and
// format text. Metering point numbers are ids and may be logged (W-261008-ca2).
var neverLogNames = regexp.MustCompile(`(?i)firstname|lastname|name1|name2|email|iban|password|passwd|secret|token|phone|accountholder|credential|authorization|bearer|creds|basicauth|payload|body|username|user_id|subject|\bsub\b`)

// neverLogAllowed: key "<file>: <hit>", without a line number, so an entry
// survives edits around it but not a change of what is logged.
var neverLogAllowed = map[string]string{
	`api/middleware/tokenVerifier.go: field key "JWT-Token"`: "a constant label as the value of the field error, no token",
	"mqtt/messageBroker.go: name token":                      "the paho delivery token, only its Error() is logged",
	"eda/edaSubscription.go: %+v of []eda.responseCodesPerMeter": "metering point, response codes, consent end and a " +
		"pointer to the consent id: no personal data, metering point numbers may be logged",
	`api/middleware/tokenVerifier.go: name in the format text "Authorization: %v"`: "the err is ErrNoAuthorization or " +
		"ErrInvalidAuthSchema, constant texts, never the header",
	"cmd/mailtemplate-import/main.go: name Body":    "operator tool, prints the template on its own terminal, not a service log",
	"cmd/mailtemplate-import/main.go: name Subject": "operator tool, prints the template on its own terminal, not a service log",
	"eda/edaSubscription.go: name Payload":          "only msg.Payload.ConversationId is logged; the struct itself is caught as a whole value",
	"mqtt/subscriptionHandler.go: name Payload":     "only msg.Payload.ConversationId is logged",
	"mqtt/messageBroker.go: name Payload":           "only len(msg.Payload()) is logged",
}

func TestNeverLog(t *testing.T) {
	var failed []string
	used := map[string]bool{}
	for _, h := range scanPackages(t, "./...") {
		if _, ok := neverLogAllowed[h.key]; ok {
			used[h.key] = true
			continue
		}
		failed = append(failed, fmt.Sprintf("%s:%d: %s", h.file, h.line, h.what))
	}
	for k := range neverLogAllowed {
		if !used[k] {
			failed = append(failed, "stale neverLogAllowed entry: "+k)
		}
	}
	assert.Empty(t, failed, "never-log rule, see the head of this file:\n%s", strings.Join(failed, "\n"))
}

// The scan itself: every line marked "// want" in the fixture is a hit and no other is.
func TestNeverLog_FindsTheFixture(t *testing.T) {
	const fixture = "logging/testdata/neverlog/bad.go"
	src, err := os.ReadFile(filepath.FromSlash("testdata/neverlog/bad.go"))
	require.NoError(t, err)
	want := map[int]bool{}
	for i, l := range strings.Split(string(src), "\n") {
		if strings.HasSuffix(strings.TrimSpace(l), "// want") {
			want[i+1] = true
		}
	}
	require.NotEmpty(t, want)

	got := map[int]bool{}
	for _, h := range scanPackages(t, "./logging/testdata/neverlog") {
		require.Equal(t, fixture, h.file)
		got[h.line] = true
	}
	assert.Equal(t, want, got)
}

type neverLogHit struct {
	key, file, what string
	line            int
}

type listedPackage struct {
	ImportPath, Dir, Export string
	GoFiles                 []string
	Module                  *struct{ Main bool }
}

func scanPackages(t *testing.T, pattern string) []neverLogHit {
	t.Helper()
	root, err := filepath.Abs("..")
	require.NoError(t, err)
	cmd := exec.Command("go", "list", "-export", "-deps", "-json=ImportPath,Dir,Export,GoFiles,Module", pattern)
	cmd.Dir = root
	out, err := cmd.Output()
	var failed *exec.ExitError
	if errors.As(err, &failed) {
		t.Logf("go list: %s", failed.Stderr)
	}
	require.NoError(t, err, "go list")

	exports := map[string]string{}
	var own []listedPackage
	for dec := json.NewDecoder(strings.NewReader(string(out))); ; {
		var p listedPackage
		if err := dec.Decode(&p); err == io.EOF {
			break
		} else {
			require.NoError(t, err)
		}
		exports[p.ImportPath] = p.Export
		if p.Module != nil && p.Module.Main {
			own = append(own, p)
		}
	}

	fset := token.NewFileSet()
	imp := importer.ForCompiler(fset, "gc", func(path string) (io.ReadCloser, error) { return os.Open(exports[path]) })
	s := &scanner{fset: fset, root: root}
	for _, p := range own {
		var files []*ast.File
		for _, name := range p.GoFiles {
			f, err := parser.ParseFile(fset, filepath.Join(p.Dir, name), nil, parser.ParseComments)
			require.NoError(t, err)
			files = append(files, f)
		}
		s.info = &types.Info{Types: map[ast.Expr]types.TypeAndValue{}, Uses: map[*ast.Ident]types.Object{}}
		_, err := (&types.Config{Importer: imp}).Check(p.ImportPath, fset, files, s.info)
		require.NoError(t, err, p.ImportPath)
		for _, f := range files {
			if !ast.IsGenerated(f) {
				ast.Inspect(f, s.visit)
			}
		}
	}
	return s.hits
}

type scanner struct {
	fset *token.FileSet
	info *types.Info
	root string
	hits []neverLogHit
}

func (s *scanner) hit(pos token.Pos, what string) {
	p := s.fset.Position(pos)
	file, _ := filepath.Rel(s.root, p.Filename)
	file = filepath.ToSlash(file)
	s.hits = append(s.hits, neverLogHit{key: file + ": " + what, file: file, what: what, line: p.Line})
}

func (s *scanner) visit(n ast.Node) bool {
	call, ok := n.(*ast.CallExpr)
	if !ok {
		return true
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return true
	}
	name := sel.Sel.Name
	switch s.packageOf(sel) {
	case "github.com/sirupsen/logrus", "log":
		switch {
		case name == "WithField" || name == "WithFields":
			s.fields(call.Args)
		case name == "Logf":
			s.formatted(call.Args[1:])
		case logMethods[strings.TrimSuffix(strings.TrimSuffix(name, "ln"), "f")]:
			if strings.HasSuffix(name, "f") {
				s.formatted(call.Args)
			} else {
				s.plain(call.Args)
			}
		}
	case "log/slog":
		if logMethods[strings.TrimSuffix(name, "Context")] || name == "With" || name == "LogAttrs" {
			s.fields(call.Args)
		}
	case "fmt":
		switch name {
		case "Print", "Println":
			s.plain(call.Args)
		case "Printf":
			s.formatted(call.Args)
		}
	}
	return true
}

// packageOf is the package of a function, or of the type of a method's receiver.
func (s *scanner) packageOf(sel *ast.SelectorExpr) string {
	if id, ok := sel.X.(*ast.Ident); ok {
		if pn, ok := s.info.Uses[id].(*types.PkgName); ok {
			return pn.Imported().Path()
		}
	}
	t := s.info.Types[sel.X].Type
	if p, ok := t.(*types.Pointer); ok {
		t = p.Elem()
	}
	if n, ok := t.(*types.Named); ok && n.Obj().Pkg() != nil {
		return n.Obj().Pkg().Path()
	}
	return ""
}

var logMethods = map[string]bool{
	"Print": true, "Trace": true, "Debug": true, "Info": true, "Warn": true, "Warning": true,
	"Error": true, "Fatal": true, "Panic": true, "Log": true,
}

var (
	formatVerb = regexp.MustCompile(`%([-+# 0]*)[0-9]*(?:\.[0-9]+)?([a-zA-Z%])`)
	// a name right before a verb: "password=%s", "email: %v"
	beforeVerb = regexp.MustCompile(`(?i)(?:` + neverLogNames.String() + `)[a-z]*[ =:'"(-]{0,3}%[^%]`)
	fieldKey   = regexp.MustCompile(`^"[\w.-]+"$`)
)

func (s *scanner) plain(args []ast.Expr) {
	for _, a := range args {
		s.names(a)
		s.whole(a, "bare argument")
	}
}

func (s *scanner) formatted(args []ast.Expr) {
	format, values := args[0], args[1:]
	for _, a := range values {
		s.names(a)
	}
	tv := s.info.Types[format]
	if tv.Value == nil || tv.Value.Kind() != constant.String {
		for _, a := range values {
			s.whole(a, "argument of a variable format")
		}
		return
	}
	text := constant.StringVal(tv.Value)
	if m := beforeVerb.FindString(text); m != "" {
		s.hit(format.Pos(), fmt.Sprintf("name in the format text %q", m))
	}
	i := 0
	for _, m := range formatVerb.FindAllStringSubmatch(text, -1) {
		if m[2] == "%" || i >= len(values) {
			continue
		}
		if m[2] != "T" && m[2] != "p" {
			s.whole(values[i], "%"+m[1]+m[2])
			s.dumpedInterface(values[i], m[1])
		}
		i++
	}
}

// fields looks at keys and values of WithField(s) and slog calls. The message
// of a slog call is text; a literal counts only if it looks like a key.
func (s *scanner) fields(args []ast.Expr) {
	for _, a := range args {
		ast.Inspect(a, func(n ast.Node) bool {
			if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING &&
				fieldKey.MatchString(lit.Value) && neverLogNames.MatchString(lit.Value) {
				s.hit(lit.Pos(), "field key "+lit.Value)
			}
			return true
		})
		s.names(a)
		if lit, ok := a.(*ast.CompositeLit); ok {
			for _, e := range lit.Elts {
				if kv, ok := e.(*ast.KeyValueExpr); ok {
					s.whole(kv.Value, "field value")
				}
			}
		} else {
			s.whole(a, "field value")
		}
	}
}

// names reports identifiers (variables, fields, methods) that match.
func (s *scanner) names(e ast.Expr) {
	ast.Inspect(e, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && neverLogNames.MatchString(id.Name) {
			if _, pkg := s.info.Uses[id].(*types.PkgName); !pkg {
				s.hit(id.Pos(), "name "+id.Name)
			}
		}
		return true
	})
}

func (s *scanner) whole(e ast.Expr, how string) {
	if t := s.info.Types[e].Type; t != nil && printsWhole(t) {
		s.hit(e.Pos(), how+" of "+types.TypeString(t, func(p *types.Package) string { return p.Name() }))
	}
}

// dumpedInterface: %+v and %#v of an interface can hold anything, also a client
// with its options. A plain %v is left alone, it is too common for a bare value.
func (s *scanner) dumpedInterface(e ast.Expr, flags string) {
	t := s.info.Types[e].Type
	if t == nil || !strings.ContainsAny(flags, "+#") || hasPrintMethod(t) {
		return
	}
	if _, ok := t.Underlying().(*types.Interface); ok {
		s.hit(e.Pos(), "%"+flags+"v of "+types.TypeString(t, func(p *types.Package) string { return p.Name() }))
	}
}

func printsWhole(t types.Type) bool {
	if hasPrintMethod(t) {
		return false
	}
	if n, ok := t.(*types.Named); ok && n.Obj().Pkg() != nil &&
		n.Obj().Pkg().Path() == "net/http" && n.Obj().Name() == "Header" {
		return true // holds Authorization and cookies
	}
	switch u := t.Underlying().(type) {
	case *types.Struct:
		return true
	case *types.Pointer:
		return printsWhole(u.Elem())
	case *types.Slice:
		return printsWhole(u.Elem())
	case *types.Array:
		return printsWhole(u.Elem())
	case *types.Map:
		return printsWhole(u.Elem())
	}
	return false
}

// hasPrintMethod: an error or Stringer prints itself, not its fields.
func hasPrintMethod(t types.Type) bool {
	for _, name := range []string{"Error", "String"} {
		if obj, _, _ := types.LookupFieldOrMethod(t, true, nil, name); obj != nil {
			if f, ok := obj.(*types.Func); ok && f.Signature().Params().Len() == 0 {
				return true
			}
		}
	}
	return false
}
