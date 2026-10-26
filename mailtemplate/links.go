package mailtemplate

import (
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Link problem keys, the same in mail-link-cases.json and eegfaktura-billing.
const (
	LinkIp             = "ip"
	LinkShortener      = "shortener"
	LinkTextMismatch   = "text-mismatch"
	LinkTooManyLinks   = "too-many-links"
	LinkTooManyDomains = "too-many-domains"
)

// LinkProblem is one broken link rule of a mail text, with the message the
// community reads.
type LinkProblem struct {
	Key     string
	Message string
}

// CheckLinks checks the links (<a href>) of a sanitised body against the link
// rules of mail-html-policy.json: no IP address, no URL shortener, a link text
// that shows an address shows the target's, at most maxLinks links and
// maxLinkDomains main domains. The fixed footer is not part of the body. It returns
// one problem per broken rule, nil when the text is fine.
func CheckLinks(body string) []LinkProblem {
	nodes, err := parseFragment(body)
	if err != nil {
		return nil
	}
	var ips, shorteners, mismatches, domains []string
	seen := map[string]bool{}
	add := func(list *[]string, kind, v string) {
		if !seen[kind+" "+v] {
			seen[kind+" "+v] = true
			*list = append(*list, v)
		}
	}
	links := 0
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type != html.ElementNode {
				continue
			}
			href := strings.TrimSpace(attr(c, "href"))
			if c.DataAtom != atom.A || href == "" {
				walk(c)
				continue
			}
			links++
			u, err := url.Parse(href)
			if err != nil {
				continue // the sanitiser keeps parseable URLs only
			}
			linkText := strings.TrimSpace(strings.ReplaceAll(textOf(c), " ", " "))
			if strings.EqualFold(u.Scheme, "mailto") {
				if !textMatchesMailto(linkText, u) {
					add(&mismatches, "text", "„"+linkText+"“")
				}
				continue
			}
			host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
			switch {
			case isIPHost(host):
				add(&ips, "ip", host)
			case isBlockedHost(host):
				add(&shorteners, "short", host)
			}
			add(&domains, "domain", mainDomain(host))
			if !textMatchesHost(linkText, host) {
				add(&mismatches, "text", "„"+linkText+"“")
			}
		}
	}
	walk(wrap(nodes))

	limits := policy.Links
	var out []LinkProblem
	if len(ips) > 0 {
		out = append(out, LinkProblem{LinkIp, "Links zu IP-Adressen sind nicht erlaubt: " + strings.Join(ips, ", ")})
	}
	if len(shorteners) > 0 {
		out = append(out, LinkProblem{LinkShortener, "Links zu Kurz-Link-Diensten sind nicht erlaubt: " + strings.Join(shorteners, ", ")})
	}
	if len(mismatches) > 0 {
		out = append(out, LinkProblem{LinkTextMismatch, "Der Linktext zeigt eine andere Adresse als das Ziel: " + strings.Join(mismatches, ", ")})
	}
	if links > limits.MaxLinks {
		out = append(out, LinkProblem{LinkTooManyLinks, fmt.Sprintf("Höchstens %d Links pro Mail (jetzt %d).", limits.MaxLinks, links)})
	}
	if len(domains) > limits.MaxLinkDomains {
		out = append(out, LinkProblem{LinkTooManyDomains, fmt.Sprintf("Links zu höchstens %d verschiedenen Domains pro Mail (jetzt %d: %s).",
			limits.MaxLinkDomains, len(domains), strings.Join(domains, ", "))})
	}
	return out
}

// LinkErrors returns the link problems of a sanitised body as validation errors of bodyHtml.
func LinkErrors(body string) []*ValidationError {
	var out []*ValidationError
	for _, p := range CheckLinks(body) {
		out = append(out, &ValidationError{Field: "bodyHtml", Message: p.Message})
	}
	return out
}

// isIPHost reports whether a host is an IP address: IPv6 or IPv4, also in the
// forms browsers accept (a last label of digits or 0x-hex, e.g. 3232235777),
// or a single label of hex digits.
func isIPHost(host string) bool {
	if net.ParseIP(host) != nil {
		return true
	}
	labels := strings.Split(host, ".")
	last := labels[len(labels)-1]
	if isNumber(last) {
		return true
	}
	return len(labels) == 1 && last != "" && strings.Trim(last, "0123456789abcdef") == ""
}

func isNumber(s string) bool {
	if s == "" {
		return false
	}
	if hex, ok := strings.CutPrefix(s, "0x"); ok {
		return strings.Trim(hex, "0123456789abcdef") == ""
	}
	return strings.Trim(s, "0123456789") == ""
}

// isBlockedHost reports whether a host is a URL shortener of the policy or one of its subdomains.
func isBlockedHost(host string) bool {
	for _, b := range policy.Links.BlockedHosts {
		if host == b || strings.HasSuffix(host, "."+b) {
			return true
		}
	}
	return false
}

// domainOf is the host a link text is compared with: lower case, without a leading www.
func domainOf(host string) string {
	return strings.TrimPrefix(strings.TrimSuffix(strings.ToLower(host), "."), "www.")
}

// mainDomain is the domain a link counts for: the last two labels of the host,
// the last three under a second-level suffix of the policy (e.g. co.at). An IP
// address counts as itself.
func mainDomain(host string) string {
	host = domainOf(host)
	if isIPHost(host) {
		return host
	}
	labels := strings.Split(host, ".")
	n := 2
	if len(labels) >= 3 {
		suffix := strings.Join(labels[len(labels)-2:], ".")
		for _, s := range policy.Links.SecondLevelSuffixes {
			if suffix == s {
				n = 3
			}
		}
	}
	if len(labels) <= n {
		return host
	}
	return strings.Join(labels[len(labels)-n:], ".")
}

// isFileName reports whether a host-like text ends in a file extension of the policy (anleitung.pdf).
func isFileName(host string) bool {
	labels := strings.Split(strings.ToLower(host), ".")
	last := labels[len(labels)-1]
	for _, ext := range policy.Links.FileExtensions {
		if last == ext {
			return true
		}
	}
	return false
}

// bareDomain is a link text like example.at or example.at/pfad.
var bareDomain = regexp.MustCompile(`^[\p{L}\p{N}-]+(\.[\p{L}\p{N}-]+)*\.\p{L}{2,}\.?(:[0-9]+)?([/?#]\S*)?$`)

// mailAddress is a link text like office@example.at.
var mailAddress = regexp.MustCompile(`^[^\s@<>]+@[^\s@<>]+$`)

// textHost returns the host a link text shows; ok=false when the text is no web address.
func textHost(text string) (host string, ok bool) {
	lower := strings.ToLower(text)
	switch {
	case strings.Contains(text, "://"):
	case strings.HasPrefix(lower, "www.") || bareDomain.MatchString(text):
		text = "https://" + text
	default:
		return "", false
	}
	u, err := url.Parse(text)
	if err != nil {
		return "", true // shows an address that cannot be compared
	}
	if !strings.Contains(lower, "://") && isFileName(u.Hostname()) {
		return "", false
	}
	return domainOf(u.Hostname()), true
}

// textMatchesHost: a link text that shows a web address shows the link's host;
// one that shows an e-mail address never matches a web link.
func textMatchesHost(text, host string) bool {
	if mailAddress.MatchString(strings.TrimPrefix(strings.ToLower(text), "mailto:")) {
		return false
	}
	shown, ok := textHost(text)
	return !ok || (shown != "" && shown == domainOf(host))
}

// textMatchesMailto: a link text that shows an e-mail address shows the
// link's address; one that shows a web address never matches a mailto link.
func textMatchesMailto(text string, u *url.URL) bool {
	if _, ok := textHost(text); ok {
		return false
	}
	shown := strings.TrimPrefix(strings.ToLower(text), "mailto:")
	if !mailAddress.MatchString(shown) {
		return true
	}
	addr, _, _ := strings.Cut(u.Opaque, "?")
	if a, err := url.PathUnescape(addr); err == nil {
		addr = a
	}
	return strings.EqualFold(strings.TrimSpace(addr), shown)
}

// textOf returns the text content of n.
func textOf(n *html.Node) string {
	var b strings.Builder
	var rec func(*html.Node)
	rec = func(n *html.Node) {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.TextNode {
				b.WriteString(c.Data)
			}
			rec(c)
		}
	}
	rec(n)
	return b.String()
}
