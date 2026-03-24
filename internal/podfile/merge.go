package podfile

import "strings"

// Merge combines a base Podfile with a child RawPodfile. Child values take
// precedence. Lists append (unless the child's bang flag is set). Maps merge
// with child winning on key conflict. Scalars: child overrides if non-zero.
// Neither base nor child is mutated; a new Podfile is returned.
func Merge(base *Podfile, child *RawPodfile) *Podfile {
	out := *base

	// Scalars: child wins if non-empty
	if child.Base != "" {
		out.Base = child.Base
	}
	if child.Shell != "" {
		out.Shell = child.Shell
	}
	if child.Mount != "" {
		out.Mount = child.Mount
	}
	if child.Mode != "" {
		out.Mode = child.Mode
	}
	if child.Workspace != "" {
		out.Workspace = child.Workspace
	}
	if child.Ports.Strategy != "" {
		out.Ports.Strategy = child.Ports.Strategy
	}

	// Name: always child (not inherited)
	out.Name = child.Name

	// Extends: cleared (already resolved)
	out.Extends = ""

	// Dotfiles: child replaces entirely if set
	if child.Dotfiles != nil {
		out.Dotfiles = child.Dotfiles
	}

	// Resources: per-field override
	if child.Resources.CPUs > 0 {
		out.Resources.CPUs = child.Resources.CPUs
	}
	if child.Resources.Memory != "" {
		out.Resources.Memory = child.Resources.Memory
	}

	// Packages
	if child.Flags.PackagesReplace {
		out.Packages = cloneStrings(child.Packages)
	} else if len(child.Packages) > 0 {
		out.Packages = mergeStringList(base.Packages, child.Packages)
	} else {
		out.Packages = cloneStrings(base.Packages)
	}

	// Env
	if child.Flags.EnvReplace {
		out.Env = cloneMap(child.Env)
	} else {
		out.Env = mergeMaps(base.Env, child.Env)
	}

	// ExtraCommands
	if child.Flags.ExtraCommandsReplace {
		out.ExtraCommands = cloneStrings(child.ExtraCommands)
	} else if len(child.ExtraCommands) > 0 {
		out.ExtraCommands = append(cloneStrings(base.ExtraCommands), child.ExtraCommands...)
	} else {
		out.ExtraCommands = cloneStrings(base.ExtraCommands)
	}

	// Lifecycle hooks: concatenate by default, replace with bang
	if child.Flags.OnCreateReplace {
		out.OnCreate = child.OnCreate
	} else {
		out.OnCreate = concatHooks(base.OnCreate, child.OnCreate)
	}
	if child.Flags.OnStartReplace {
		out.OnStart = child.OnStart
	} else {
		out.OnStart = concatHooks(base.OnStart, child.OnStart)
	}

	// Services
	if child.Flags.ServicesReplace {
		out.Services = cloneServices(child.Services)
	} else {
		out.Services = mergeServices(base.Services, child.Services)
	}

	// Repos
	if child.Flags.ReposReplace {
		out.Repos = cloneRepos(child.Repos)
	} else {
		out.Repos = mergeRepos(base.Repos, child.Repos)
	}

	// Ports.Expose
	out.Ports.Expose = dedupeInts(append(cloneInts(base.Ports.Expose), child.Ports.Expose...))

	return &out
}

// mergeStringList handles package-style list merging with !item removal.
func mergeStringList(base, child []string) []string {
	removals := map[string]bool{}
	var additions []string
	for _, item := range child {
		if strings.HasPrefix(item, "!") {
			removals[strings.TrimPrefix(item, "!")] = true
		} else {
			additions = append(additions, item)
		}
	}

	seen := map[string]bool{}
	var result []string
	for _, item := range base {
		if removals[item] || seen[item] {
			continue
		}
		seen[item] = true
		result = append(result, item)
	}
	for _, item := range additions {
		if seen[item] {
			continue
		}
		seen[item] = true
		result = append(result, item)
	}
	return result
}

func mergeMaps(base, child map[string]string) map[string]string {
	out := make(map[string]string)
	for k, v := range base {
		out[k] = v
	}
	for k, v := range child {
		if v == "" {
			delete(out, k)
		} else {
			out[k] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func mergeServices(base, child []ServiceConfig) []ServiceConfig {
	removals := map[string]bool{}
	replacements := map[string]ServiceConfig{}
	var additions []ServiceConfig

	for _, svc := range child {
		if strings.HasPrefix(svc.Name, "!") {
			removals[strings.TrimPrefix(svc.Name, "!")] = true
		} else {
			found := false
			for _, bsvc := range base {
				if bsvc.Name == svc.Name {
					replacements[svc.Name] = svc
					found = true
					break
				}
			}
			if !found {
				additions = append(additions, svc)
			}
		}
	}

	var result []ServiceConfig
	for _, svc := range base {
		if removals[svc.Name] {
			continue
		}
		if repl, ok := replacements[svc.Name]; ok {
			result = append(result, repl)
		} else {
			result = append(result, svc)
		}
	}
	result = append(result, additions...)
	return result
}

func mergeRepos(base, child []RepoConfig) []RepoConfig {
	removals := map[string]bool{}
	updates := map[string]RepoConfig{}
	var additions []RepoConfig

	for _, repo := range child {
		if strings.HasPrefix(repo.URL, "!") {
			removals[strings.TrimPrefix(repo.URL, "!")] = true
		} else {
			found := false
			for _, brepo := range base {
				if brepo.URL == repo.URL {
					merged := brepo
					if repo.Path != "" {
						merged.Path = repo.Path
					}
					if repo.Branch != "" {
						merged.Branch = repo.Branch
					}
					updates[repo.URL] = merged
					found = true
					break
				}
			}
			if !found {
				additions = append(additions, repo)
			}
		}
	}

	var result []RepoConfig
	for _, repo := range base {
		if removals[repo.URL] {
			continue
		}
		if upd, ok := updates[repo.URL]; ok {
			result = append(result, upd)
		} else {
			result = append(result, repo)
		}
	}
	result = append(result, additions...)
	return result
}

func concatHooks(base, child string) string {
	if base == "" {
		return child
	}
	if child == "" {
		return base
	}
	return base + "\n" + child
}

func cloneStrings(s []string) []string {
	if s == nil {
		return nil
	}
	out := make([]string, len(s))
	copy(out, s)
	return out
}

func cloneMap(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func cloneInts(s []int) []int {
	if s == nil {
		return nil
	}
	out := make([]int, len(s))
	copy(out, s)
	return out
}

func cloneServices(svcs []ServiceConfig) []ServiceConfig {
	if svcs == nil {
		return nil
	}
	out := make([]ServiceConfig, len(svcs))
	copy(out, svcs)
	return out
}

func cloneRepos(repos []RepoConfig) []RepoConfig {
	if repos == nil {
		return nil
	}
	out := make([]RepoConfig, len(repos))
	copy(out, repos)
	return out
}

func dedupeInts(s []int) []int {
	if len(s) == 0 {
		return nil
	}
	seen := map[int]bool{}
	var out []int
	for _, v := range s {
		if seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}
