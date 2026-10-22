package mailtemplate

import (
	_ "embed"
	"encoding/json"
	"regexp"
	"sort"
	"strings"

	"github.com/microcosm-cc/bluemonday"
)

// mail-html-policy.json is shared byte-identically with eegfaktura-billing;
// change both together.
//
//go:embed mail-html-policy.json
var policyJSON []byte

// htmlPolicy is the parsed mail-html-policy.json.
type htmlPolicy struct {
	Version    int                 `json:"version"`
	Elements   map[string][]string `json:"elements"`
	URLSchemes map[string][]string `json:"urlSchemes"`
	Styles     map[string][]string `json:"styles"`
	Required   map[string][]string `json:"required"`
	LinkRel    string              `json:"linkRel"`
}

var policy = mustLoadPolicy(policyJSON)

func mustLoadPolicy(data []byte) *htmlPolicy {
	var p htmlPolicy
	if err := json.Unmarshal(data, &p); err != nil {
		panic("mail-html-policy.json: " + err.Error())
	}
	if len(p.Elements) == 0 {
		panic("mail-html-policy.json: no elements")
	}
	return &p
}

// AllowedElements returns the HTML elements the policy allows.
func AllowedElements() []string {
	out := make([]string, 0, len(policy.Elements))
	for el := range policy.Elements {
		out = append(out, el)
	}
	sort.Strings(out)
	return out
}

// allowsAttr reports whether the policy allows attribute attr on element el.
func (p *htmlPolicy) allowsAttr(el, attr string) bool {
	for _, a := range p.Elements[el] {
		if a == attr {
			return true
		}
	}
	return false
}

// urlSchemes returns the allowed URL schemes of el.attr, nil if it is no URL attribute.
func (p *htmlPolicy) urlSchemes(el, attr string) []string {
	return p.URLSchemes[el+"."+attr]
}

// requiredAttrs returns the attributes el cannot exist without: the "required"
// list and its URL attributes (a link without href, an image without src).
func (p *htmlPolicy) requiredAttrs(el string) []string {
	req := append([]string{}, p.Required[el]...)
	for key := range p.URLSchemes {
		if e, a, ok := strings.Cut(key, "."); ok && e == el {
			req = append(req, a)
		}
	}
	return req
}

// bluemondayPolicy builds the first sanitising pass from the policy file. It
// removes everything outside the allowlist (including script/style content);
// the second pass (sanitizer.clean) applies what bluemonday cannot express per
// attribute: URL schemes per element, required attributes, placeholders, files.
func (p *htmlPolicy) bluemondayPolicy() *bluemonday.Policy {
	bm := bluemonday.NewPolicy()
	bm.RequireParseableURLs(true)
	bm.AllowRelativeURLs(false)

	schemes := map[string]bool{}
	for _, list := range p.URLSchemes {
		for _, s := range list {
			schemes[s] = true
		}
	}
	all := make([]string, 0, len(schemes))
	for s := range schemes {
		all = append(all, s)
	}
	sort.Strings(all)
	bm.AllowURLSchemes(all...)

	// style passes bluemonday unchanged on the elements that may carry it; the
	// second pass keeps only the declarations of the policy (cleanStyle), which
	// also copes with "text-align:center;;" or upper case values
	for el, attrs := range p.Elements {
		bm.AllowElements(el)
		for _, attr := range attrs {
			bm.AllowAttrs(attr).OnElements(el)
		}
	}
	return bm
}

// imgWidth is the accepted value of img[width]: pixels or percent.
var imgWidth = regexp.MustCompile(`^[0-9]{1,4}%?$`)

// cleanStyle keeps only the declarations of a style attribute the policy allows.
func (p *htmlPolicy) cleanStyle(style string) string {
	var kept []string
	for _, decl := range strings.Split(style, ";") {
		prop, val, ok := strings.Cut(decl, ":")
		if !ok {
			continue
		}
		prop = strings.ToLower(strings.TrimSpace(prop))
		val = strings.ToLower(strings.TrimSpace(val))
		for _, allowed := range p.Styles[prop] {
			if val == allowed {
				kept = append(kept, prop+": "+val)
				break
			}
		}
	}
	return strings.Join(kept, "; ")
}
