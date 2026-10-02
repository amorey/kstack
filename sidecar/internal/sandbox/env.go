// Copyright 2026 The Kstack Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package sandbox

import (
	"slices"
	"strings"
)

// EnvRule matches an environment variable no sandboxed run may hold: by its
// name, or every name starting with Prefix but those in Except.
type EnvRule struct {
	Name   string
	Prefix string
	Except []string
}

// neverEnv is what no sandboxed run may hold: what reaches past the sandbox,
// what changes what a program loads, what a shell runs on its own, and every
// AWS variable. A step that sets one of these names excepts it here, in its
// own change, with a test that pins the value it sets.
var neverEnv = []EnvRule{
	{Name: "SSH_AUTH_SOCK"}, {Name: "GITHUB_TOKEN"}, {Name: "GH_TOKEN"}, {Name: "DOCKER_HOST"},
	{Name: "DYLD_LIBRARY_PATH"}, {Name: "DYLD_INSERT_LIBRARIES"}, {Name: "DYLD_FRAMEWORK_PATH"},
	{Name: "LD_LIBRARY_PATH"}, {Name: "LD_PRELOAD"}, {Name: "LD_AUDIT"},
	{Name: "BASH_ENV"}, {Name: "ENV"}, {Name: "PROMPT_COMMAND"},
	{Prefix: "AWS_"},
}

// NeverEnv is the rules, a copy each call.
func NeverEnv() []EnvRule {
	rules := slices.Clone(neverEnv)
	for i := range rules {
		rules[i].Except = slices.Clone(rules[i].Except)
	}
	return rules
}

// Unpassable reports whether name matches a rule.
func Unpassable(name string) bool { return unpassable(neverEnv, name) }

func unpassable(rules []EnvRule, name string) bool {
	return slices.ContainsFunc(rules, func(r EnvRule) bool {
		if r.Prefix != "" {
			return strings.HasPrefix(name, r.Prefix) && !slices.Contains(r.Except, name)
		}
		return name == r.Name
	})
}
