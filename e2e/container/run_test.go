package container

import (
	"fmt"
	"strings"
	"testing"

	"github.com/docker/cli/e2e/internal/fixtures"
	"github.com/docker/cli/internal/test/environment"
	"gotest.tools/v3/assert"
	is "gotest.tools/v3/assert/cmp"
	"gotest.tools/v3/golden"
	"gotest.tools/v3/icmd"
	"gotest.tools/v3/skip"
)

const registryPrefix = "registry:5000"

func TestRunAttachedFromRemoteImageAndRemove(t *testing.T) {
	skip.If(t, environment.RemoteDaemon())

	// Digests in golden file are linux/amd64 specific.
	// TODO: Fix this test and make it work on all platforms.
	environment.SkipIfNotPlatform(t, "linux/amd64")

	image := createRemoteImage(t)

	result := icmd.RunCommand("docker", "run", "--rm", image,
		"echo", "this", "is", "output")

	result.Assert(t, icmd.Success)
	assert.Check(t, is.Equal("this is output\n", result.Stdout()))
	golden.Assert(t, result.Stderr(), "run-attached-from-remote-and-remove.golden")
}

func TestRunWithContentTrust(t *testing.T) {
	skip.If(t, environment.RemoteDaemon())

	dir := fixtures.SetupConfigFile(t)
	defer dir.Remove()
	image := fixtures.CreateMaskedTrustedRemoteImage(t, registryPrefix, "trust-run", "latest")

	defer func() {
		icmd.RunCommand("docker", "image", "rm", image).Assert(t, icmd.Success)
	}()

	result := icmd.RunCmd(
		icmd.Command("docker", "run", image),
		fixtures.WithConfig(dir.Path()),
		fixtures.WithTrust,
		fixtures.WithNotary,
	)
	result.Assert(t, icmd.Expected{
		Err: fmt.Sprintf("Tagging %s@sha", image[:len(image)-7]),
	})
}

func TestUntrustedRun(t *testing.T) {
	dir := fixtures.SetupConfigFile(t)
	defer dir.Remove()
	image := registryPrefix + "/alpine:untrusted"
	// tag the image and upload it to the private registry
	icmd.RunCommand("docker", "tag", fixtures.AlpineImage, image).Assert(t, icmd.Success)
	defer func() {
		icmd.RunCommand("docker", "image", "rm", image).Assert(t, icmd.Success)
	}()

	// try trusted run on untrusted tag
	result := icmd.RunCmd(
		icmd.Command("docker", "run", image),
		fixtures.WithConfig(dir.Path()),
		fixtures.WithTrust,
		fixtures.WithNotary,
	)
	result.Assert(t, icmd.Expected{
		ExitCode: 125,
		Err:      "does not have trust data for",
	})
}

func TestTrustedRunFromBadTrustServer(t *testing.T) {
	evilImageName := registryPrefix + "/evil-alpine:latest"
	dir := fixtures.SetupConfigFile(t)
	defer dir.Remove()

	// tag the image and upload it to the private registry
	icmd.RunCmd(icmd.Command("docker", "tag", fixtures.AlpineImage, evilImageName),
		fixtures.WithConfig(dir.Path()),
	).Assert(t, icmd.Success)
	icmd.RunCmd(icmd.Command("docker", "image", "push", evilImageName),
		fixtures.WithConfig(dir.Path()),
		fixtures.WithPassphrase("root_password", "repo_password"),
		fixtures.WithTrust,
		fixtures.WithNotary,
	).Assert(t, icmd.Success)
	icmd.RunCmd(icmd.Command("docker", "image", "rm", evilImageName)).Assert(t, icmd.Success)

	// try run
	icmd.RunCmd(icmd.Command("docker", "run", evilImageName),
		fixtures.WithConfig(dir.Path()),
		fixtures.WithTrust,
		fixtures.WithNotary,
	).Assert(t, icmd.Success)
	icmd.RunCmd(icmd.Command("docker", "image", "rm", evilImageName)).Assert(t, icmd.Success)

	// init a client with the evil-server and a new trust dir
	evilNotaryDir := fixtures.SetupConfigWithNotaryURL(t, "evil-test", fixtures.EvilNotaryURL)
	defer evilNotaryDir.Remove()

	// tag the same image and upload it to the private registry but signed with evil notary server
	icmd.RunCmd(icmd.Command("docker", "tag", fixtures.AlpineImage, evilImageName),
		fixtures.WithConfig(evilNotaryDir.Path()),
	).Assert(t, icmd.Success)
	icmd.RunCmd(icmd.Command("docker", "image", "push", evilImageName),
		fixtures.WithConfig(evilNotaryDir.Path()),
		fixtures.WithPassphrase("root_password", "repo_password"),
		fixtures.WithTrust,
		fixtures.WithNotaryServer(fixtures.EvilNotaryURL),
	).Assert(t, icmd.Success)
	icmd.RunCmd(icmd.Command("docker", "image", "rm", evilImageName)).Assert(t, icmd.Success)

	// try running with the original client from the evil notary server. This should failed
	// because the new root is invalid
	icmd.RunCmd(icmd.Command("docker", "run", evilImageName),
		fixtures.WithConfig(dir.Path()),
		fixtures.WithTrust,
		fixtures.WithNotaryServer(fixtures.EvilNotaryURL),
	).Assert(t, icmd.Expected{
		ExitCode: 125,
		Err:      "could not rotate trust to a new trusted root",
	})
}

// TODO: create this with registry API instead of engine API
func createRemoteImage(t *testing.T) string {
	image := registryPrefix + "/alpine:test-run-pulls"
	icmd.RunCommand("docker", "pull", fixtures.AlpineImage).Assert(t, icmd.Success)
	icmd.RunCommand("docker", "tag", fixtures.AlpineImage, image).Assert(t, icmd.Success)
	icmd.RunCommand("docker", "push", image).Assert(t, icmd.Success)
	icmd.RunCommand("docker", "rmi", image).Assert(t, icmd.Success)
	return image
}

func TestRunWithCgroupNamespace(t *testing.T) {
	environment.SkipIfDaemonNotLinux(t)
	environment.SkipIfCgroupNamespacesNotSupported(t)

	result := icmd.RunCommand("docker", "run", "--cgroupns=private", "--rm", fixtures.AlpineImage,
		"/bin/grep", "-q", "':memory:/$'", "/proc/1/cgroup")
	result.Assert(t, icmd.Success)
}

func TestMountSubvolume(t *testing.T) {
	t.SkipNow() // TODO: Enable when testing against 1.44+
	volName := "test-volume-" + t.Name()
	icmd.RunCommand("docker", "volume", "create", volName).Assert(t, icmd.Success)

	t.Cleanup(func() {
		icmd.RunCommand("docker", "volume", "remove", "-f", volName).Assert(t, icmd.Success)
	})

	defaultMountOpts := []string{
		"type=volume",
		"src=" + volName,
		"dst=/volume",
	}

	// Populate the volume with test data.
	icmd.RunCommand("docker", "run", "--mount", strings.Join(defaultMountOpts, ","), fixtures.AlpineImage, "sh", "-c",
		"echo foo > /volume/bar.txt && "+
			"mkdir /volume/subdir && echo world > /volume/subdir/hello.txt;",
	).Assert(t, icmd.Success)

	runMount := func(cmd string, mountOpts ...string) *icmd.Result {
		mountArg := strings.Join(append(defaultMountOpts, mountOpts...), ",")
		return icmd.RunCommand("docker", "run", "--mount", mountArg, fixtures.AlpineImage, cmd, "/volume")
	}

	t.Run("subpath not exists", func(t *testing.T) {
		runMount("ls", "volume-subpath=some-path/that/doesnt-exist").Assert(t, icmd.Expected{Err: "volume's path is not accessible", ExitCode: 125})
	})
	t.Run("subdirectory mount", func(t *testing.T) {
		runMount("ls", "volume-subpath=subdir").Assert(t, icmd.Expected{Out: "hello.txt"})
	})
	t.Run("file mount", func(t *testing.T) {
		runMount("cat", "volume-subpath=bar.txt").Assert(t, icmd.Expected{Out: "foo"})
	})
}
