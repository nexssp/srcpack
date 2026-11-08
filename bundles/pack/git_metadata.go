package packcmd

import (
	"context"
	"os/exec"
	"strings"
)

type gitMetadata struct {
	Commit string
	Tag    string
	Branch string
}

const gitMetadataFormat = "%H%x00%D%x00%(describe:tags,abbrev=0)"

type (
	gitMetadataCommand  func(context.Context, string, ...string) ([]byte, error)
	gitMetadataResolver func(context.Context, string) gitMetadata
)

// resolveGitMetadata reads repository facts relative to the workspace root.
// Command failures (including a non-repository workspace or an unborn branch)
// leave the corresponding values empty rather than making packing fail.
func resolveGitMetadata(ctx context.Context, root string) gitMetadata {
	return resolveGitMetadataWith(ctx, root, runGitMetadataCommand)
}

func resolveGitMetadataWith(ctx context.Context, root string, run gitMetadataCommand) gitMetadata {
	if root == "" {
		return gitMetadata{}
	}

	data, err := run(ctx, root, "log", "-1", "--format="+gitMetadataFormat)
	if err != nil {
		return gitMetadata{}
	}
	fields := strings.SplitN(strings.TrimRight(string(data), "\r\n"), "\x00", 3)
	if len(fields) != 3 {
		return gitMetadata{}
	}

	return gitMetadata{
		Commit: strings.TrimSpace(fields[0]),
		Tag:    strings.TrimSpace(fields[2]),
		Branch: branchFromDecoration(fields[1]),
	}
}

func branchFromDecoration(refs string) string {
	for ref := range strings.SplitSeq(strings.Join(strings.Fields(refs), " "), ", ") {
		if branch, ok := strings.CutPrefix(ref, "HEAD -> "); ok {
			return branch
		}
	}
	return ""
}

func runGitMetadataCommand(ctx context.Context, root string, args ...string) ([]byte, error) {
	commandArgs := append([]string{"-C", root}, args...)
	return exec.CommandContext(ctx, "git", commandArgs...).Output()
}

func gitMetadataForResolution(ctx context.Context, plan bool, root string, resolve gitMetadataResolver) gitMetadata {
	if plan {
		return gitMetadata{}
	}
	return resolve(ctx, root)
}
