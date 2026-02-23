package filter

import (
	"regexp"

	audiciav1alpha1 "github.com/felixnotka/audicia/operator/pkg/apis/audicia.io/v1alpha1"
)

// compiledFilter is a pre-compiled filter rule.
type compiledFilter struct {
	action           audiciav1alpha1.FilterAction
	userPattern      *regexp.Regexp
	namespacePattern *regexp.Regexp
}

// Chain evaluates an ordered list of allow/deny filters. First match wins.
type Chain struct {
	filters []compiledFilter
}

// NewChain compiles the filter rules into a Chain.
func NewChain(rules []audiciav1alpha1.Filter) (*Chain, error) {
	compiled := make([]compiledFilter, 0, len(rules))
	for _, r := range rules {
		cf := compiledFilter{action: r.Action}

		if r.UserPattern != "" {
			re, err := regexp.Compile(r.UserPattern)
			if err != nil {
				return nil, err
			}
			cf.userPattern = re
		}

		if r.NamespacePattern != "" {
			re, err := regexp.Compile(r.NamespacePattern)
			if err != nil {
				return nil, err
			}
			cf.namespacePattern = re
		}

		compiled = append(compiled, cf)
	}

	return &Chain{filters: compiled}, nil
}

// Allow returns true if the event should be processed (not filtered out).
// First matching rule wins. If no rule matches, the event is allowed.
func (c *Chain) Allow(username, namespace string) bool {
	for _, f := range c.filters {
		matched := false

		if f.userPattern != nil && f.userPattern.MatchString(username) {
			matched = true
		}
		if f.namespacePattern != nil && f.namespacePattern.MatchString(namespace) {
			matched = true
		}

		if matched {
			return f.action == audiciav1alpha1.FilterActionAllow
		}
	}

	// Default: allow.
	return true
}
