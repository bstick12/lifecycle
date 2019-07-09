package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"

	"github.com/buildpack/imgutil"
	"github.com/buildpack/imgutil/local"
	"github.com/buildpack/imgutil/remote"
	"github.com/pkg/errors"

	"github.com/buildpack/lifecycle"
	"github.com/buildpack/lifecycle/cmd"
	"github.com/buildpack/lifecycle/image/auth"
)

var (
	analyzedPath string
	appDir       string
	gid          int
	groupPath    string
	layersDir    string
	repoName     string
	skipLayers   bool
	uid          int
	useDaemon    bool
	useHelpers   bool
)

func init() {
	cmd.FlagAnalyzedPath(&analyzedPath)
	cmd.FlagAppDir(&appDir)
	cmd.FlagGID(&gid)
	cmd.FlagGroupPath(&groupPath)
	cmd.FlagLayersDir(&layersDir)
	cmd.FlagUID(&uid)
	cmd.FlagUseDaemon(&useDaemon)
	cmd.FlagUseCredHelpers(&useHelpers)
	cmd.FlagSkipLayers(&skipLayers)
}

func main() {
	// suppress output from libraries, lifecycle will not use standard logger
	log.SetOutput(ioutil.Discard)

	flag.Parse()
	if flag.NArg() > 1 {
		cmd.Exit(cmd.FailErrCode(fmt.Errorf("received %d args expected 1", flag.NArg()), cmd.CodeInvalidArgs, "parse arguments"))
	}
	if flag.Arg(0) == "" {
		cmd.Exit(cmd.FailErrCode(errors.New("image argument is required"), cmd.CodeInvalidArgs, "parse arguments"))
	}
	repoName = flag.Arg(0)
	cmd.Exit(analyzer())
}

func analyzer() error {
	if useHelpers {
		if err := lifecycle.SetupCredHelpers(filepath.Join(os.Getenv("HOME"), ".docker"), repoName); err != nil {
			return cmd.FailErr(err, "setup credential helpers")
		}
	}

	group, err := lifecycle.ReadGroup(groupPath)
	if err != nil {
		return cmd.FailErr(err, "read buildpack group")
	}

	analyzer := &lifecycle.Analyzer{
		Buildpacks:   group.Group,
		AppDir:       appDir,
		LayersDir:    layersDir,
		AnalyzedPath: analyzedPath,
		Out:          log.New(os.Stdout, "", 0),
		Err:          log.New(os.Stderr, "", 0),
		UID:          uid,
		GID:          gid,
		SkipLayers:   skipLayers,
	}

	var img imgutil.Image
	if useDaemon {
		dockerClient, err := cmd.DockerClient()
		if err != nil {
			return cmd.FailErr(err, "create docker client")
		}
		img, err = local.NewImage(
			repoName,
			dockerClient,
			local.FromBaseImage(repoName),
		)
		if err != nil {
			return cmd.FailErr(err, "access previous image")
		}
	} else {
		img, err = remote.NewImage(
			repoName,
			auth.DefaultEnvKeychain(),
			remote.FromBaseImage(repoName),
		)
		if err != nil {
			return cmd.FailErr(err, "access previous image")
		}
	}

	md, err := analyzer.Analyze(img)
	if err != nil {
		return cmd.FailErrCode(err, cmd.CodeFailed, "analyze")
	}

	if err := lifecycle.WriteTOML(analyzedPath, md); err != nil {
		return errors.Wrap(err, "write analyzed.toml")
	}

	return nil
}
