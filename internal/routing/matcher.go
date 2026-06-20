package routing

import (
	"fmt"
	"regexp"
	"strings"

	"retry-proxy/internal/config"
)

type CompiledRoute struct {
	Route        config.Route
	prefixMatch  string
	regexMatch   *regexp.Regexp
	rewriteRegex []*compiledRewrite
}

type compiledRewrite struct {
	pattern     *regexp.Regexp
	replacement string
}

type Router struct {
	routes []*CompiledRoute
}

func NewRouter(routes []config.Route) (*Router, error) {
	compiled := make([]*CompiledRoute, 0, len(routes))
	for _, r := range routes {
		cr := &CompiledRoute{Route: r}

		if r.Match.Prefix != "" {
			cr.prefixMatch = r.Match.Prefix
		} else if r.Match.Regex != "" {
			rx, err := regexp.Compile(r.Match.Regex)
			if err != nil {
				return nil, fmt.Errorf("route %q regex match: %w", r.Name, err)
			}
			cr.regexMatch = rx
		}

		for i, rw := range r.Rewrite.Regex {
			rx, err := regexp.Compile(rw.Pattern)
			if err != nil {
				return nil, fmt.Errorf("route %q rewrite[%d] pattern: %w", r.Name, i, err)
			}
			cr.rewriteRegex = append(cr.rewriteRegex, &compiledRewrite{
				pattern:     rx,
				replacement: rw.Replacement,
			})
		}

		compiled = append(compiled, cr)
	}
	return &Router{routes: compiled}, nil
}

func (rt *Router) Match(path string) *CompiledRoute {
	for _, cr := range rt.routes {
		if cr.prefixMatch != "" {
			if path == cr.prefixMatch ||
				strings.HasPrefix(path, cr.prefixMatch+"/") ||
				cr.prefixMatch == "/" {
				return cr
			}
		} else if cr.regexMatch != nil {
			if cr.regexMatch.MatchString(path) {
				return cr
			}
		}
	}
	return nil
}

func (cr *CompiledRoute) RewritePath(path string) string {
	if cr.Route.Rewrite.StripPrefix && cr.prefixMatch != "" {
		path = strings.TrimPrefix(path, cr.prefixMatch)
		if path == "" {
			path = "/"
		}
	}

	for _, rw := range cr.rewriteRegex {
		path = rw.pattern.ReplaceAllString(path, rw.replacement)
	}

	return path
}
