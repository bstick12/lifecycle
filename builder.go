package lifecycle

import (
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"sort"

	"github.com/BurntSushi/toml"
)

type Builder struct {
	AppDir        string
	LayersDir     string
	PlatformDir   string
	BuildpacksDir string
	Env           BuildEnv
	Group         BuildpackGroup
	Plan          DetectPlan
	Out, Err      io.Writer
}

type BuildEnv interface {
	AddRootDir(baseDir string) error
	AddEnvDir(envDir string) error
	List() []string
}

type Process struct {
	Type    string `toml:"type"`
	Command string `toml:"command"`
}

type LaunchTOML struct {
	Processes []Process `toml:"processes"`
}

type BOMEntry struct {
	Require
	Buildpack Buildpack `toml:"buildpack"`
}

type BuildPlan struct {
	Entries []Require `toml:"entries"`
}

type BuildMetadata struct {
	Processes  []Process  `toml:"processes"`
	Buildpacks []string   `toml:"buildpacks"`
	BOM        []BOMEntry `toml:"bom"`
}

func (b *Builder) Build() (*BuildMetadata, error) {
	platformDir, err := filepath.Abs(b.PlatformDir)
	if err != nil {
		return nil, err
	}
	layersDir, err := filepath.Abs(b.LayersDir)
	if err != nil {
		return nil, err
	}
	appDir, err := filepath.Abs(b.AppDir)
	if err != nil {
		return nil, err
	}
	planDir, err := ioutil.TempDir("", "plan.")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(planDir)

	procMap := processMap{}
	plan := b.Plan
	var bom []BOMEntry
	var buildpackIDs []string
	for _, bp := range b.Group.Group {
		bpInfo, err := bp.lookup(b.BuildpacksDir)
		if err != nil {
			return nil, err
		}
		bpDirName := bp.dir()
		bpLayersDir := filepath.Join(layersDir, bpDirName)
		bpPlanDir := filepath.Join(planDir, bpDirName)
		buildpackIDs = append(buildpackIDs, bpDirName)
		if err := os.MkdirAll(bpLayersDir, 0777); err != nil {
			return nil, err
		}

		if err := os.MkdirAll(bpPlanDir, 0777); err != nil {
			return nil, err
		}
		bpPlanPath := filepath.Join(bpPlanDir, "plan.toml")
		if err := WriteTOML(bpPlanPath, plan.toBuild(bp)); err != nil {
			return nil, err
		}

		buildPath, err := filepath.Abs(filepath.Join(bpInfo.Path, "bin", "build"))
		if err != nil {
			return nil, err
		}
		cmd := exec.Command(buildPath, bpLayersDir, platformDir, bpPlanPath)
		cmd.Env = b.Env.List()
		cmd.Dir = appDir
		cmd.Stdout = b.Out
		cmd.Stderr = b.Err
		if err := cmd.Run(); err != nil {
			return nil, err
		}
		if err := setupEnv(b.Env, bpLayersDir); err != nil {
			return nil, err
		}
		var bpPlanOut BuildPlan
		if _, err := toml.DecodeFile(bpPlanPath, &bpPlanOut); err != nil {
			return nil, err
		}
		var bpBOM []BOMEntry
		plan, bpBOM = plan.filter(bp, bpPlanOut)
		bom = append(bom, bpBOM...)

		var launch LaunchTOML
		tomlPath := filepath.Join(bpLayersDir, "launch.toml")
		if _, err := toml.DecodeFile(tomlPath, &launch); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return nil, err
		}
		procMap.add(launch.Processes)
	}

	return &BuildMetadata{
		Processes:  procMap.list(),
		Buildpacks: buildpackIDs,
		BOM:        bom,
	}, nil
}

func (p DetectPlan) toBuild(bp Buildpack) BuildPlan {
	var out []Require
	for _, entry := range p.Entries {
		for _, provider := range entry.Providers {
			if provider == bp {
				out = append(out, entry.Requires...)
				break
			}
		}
	}
	return BuildPlan{Entries: out}
}

func (p DetectPlan) filter(bp Buildpack, plan BuildPlan) (DetectPlan, []BOMEntry) {
	var out []DetectPlanEntry
	for _, entry := range p.Entries {
		if !plan.has(entry) {
			out = append(out, entry)
		}
	}
	var bom []BOMEntry
	for _, entry := range plan.Entries {
		bom = append(bom, BOMEntry{Require: entry, Buildpack: bp})
	}
	return DetectPlan{Entries: out}, bom
}

func (p BuildPlan) has(entry DetectPlanEntry) bool {
	for _, buildEntry := range p.Entries {
		for _, req := range entry.Requires {
			if req.Name == buildEntry.Name {
				return true
			}
		}
	}
	return false
}

func setupEnv(env BuildEnv, layersDir string) error {
	if err := eachDir(layersDir, func(path string) error {
		if !isBuild(path + ".toml") {
			return nil
		}
		return env.AddRootDir(path)
	}); err != nil {
		return err
	}

	return eachDir(layersDir, func(path string) error {
		if !isBuild(path + ".toml") {
			return nil
		}
		if err := env.AddEnvDir(filepath.Join(path, "env")); err != nil {
			return err
		}
		return env.AddEnvDir(filepath.Join(path, "env.build"))
	})
}

func isBuild(path string) bool {
	var layerTOML struct {
		Build bool `toml:"build"`
	}
	_, err := toml.DecodeFile(path, &layerTOML)
	return err == nil && layerTOML.Build
}

type processMap map[string]Process

func (m processMap) add(l []Process) {
	for _, proc := range l {
		m[proc.Type] = proc
	}
}

func (m processMap) list() []Process {
	var keys []string
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	procs := []Process{}
	for _, key := range keys {
		procs = append(procs, m[key])
	}
	return procs
}
