package main

import (
	"errors"
	"flag"
	"io/ioutil"
	"log"
	"os"

	"github.com/buildpack/imgutil/local"

	"github.com/buildpack/lifecycle"
	"github.com/buildpack/lifecycle/cache"
	"github.com/buildpack/lifecycle/cmd"
)

var (
	cacheImageTag string
	cacheDir      string
	layersDir     string
	groupPath     string
	uid           int
	gid           int
)

func init() {
	cmd.FlagLayersDir(&layersDir)
	cmd.FlagCacheImage(&cacheImageTag)
	cmd.FlagCacheDir(&cacheDir)
	cmd.FlagGroupPath(&groupPath)
	cmd.FlagUID(&uid)
	cmd.FlagGID(&gid)
}

func main() {
	// suppress output from libraries, lifecycle will not use standard logger
	log.SetOutput(ioutil.Discard)

	flag.Parse()
	if flag.NArg() > 0 {
		cmd.Exit(cmd.FailErrCode(errors.New("received unexpected args"), cmd.CodeInvalidArgs, "parse arguments"))
	}
	if cacheImageTag == "" && cacheDir == "" {
		cmd.Exit(cmd.FailErrCode(errors.New("must supply either -image or -path"), cmd.CodeInvalidArgs, "parse arguments"))
	}
	cmd.Exit(doCache())
}

func doCache() error {
	group, err := lifecycle.ReadGroup(groupPath)
	if err != nil {
		return cmd.FailErr(err, "read buildpack group")
	}

	artifactsDir, err := ioutil.TempDir("", "lifecycle.exporter.layer")
	if err != nil {
		return cmd.FailErr(err, "create temp directory")
	}
	defer os.RemoveAll(artifactsDir)

	cacher := &lifecycle.Cacher{
		Buildpacks:   group.Group,
		ArtifactsDir: artifactsDir,
		Out:          log.New(os.Stdout, "", 0),
		Err:          log.New(os.Stderr, "", 0),
		UID:          uid,
		GID:          gid,
	}

	var cacheStore lifecycle.Cache
	if cacheImageTag != "" {
		dockerClient, err := cmd.DockerClient()
		if err != nil {
			return cmd.FailErr(err, "create docker client")
		}

		origCacheImage, err := local.NewImage(cacheImageTag, dockerClient, local.FromBaseImage(cacheImageTag))
		if err != nil {
			return cmd.FailErr(err, "access cache image")
		}

		emptyImage, err := local.NewImage(cacheImageTag, dockerClient, local.WithPreviousImage(cacheImageTag))
		if err != nil {
			return cmd.FailErr(err, "creating new cache image")
		}

		cacheStore = cache.NewImageCache(
			origCacheImage,
			emptyImage,
		)
	} else {
		var err error
		cacheStore, err = cache.NewVolumeCache(cacheDir)
		if err != nil {
			return cmd.FailErr(err, "create volume cache")
		}
	}

	if err := cacher.Cache(layersDir, cacheStore); err != nil {
		return cmd.FailErrCode(err, cmd.CodeFailed, "cache")
	}

	return nil
}
