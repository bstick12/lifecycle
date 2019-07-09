package lifecycle_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/lifecycle"
)

func TestMap(t *testing.T) {
	spec.Run(t, "Map", testMap, spec.Report(report.Terminal{}))
}

func testMap(t *testing.T, when spec.G, it spec.S) {
	when(".NewBuildpackMap", func() {
		it("should return a map of buildpacks in the provided directory", func() {
			tmpDir, err := ioutil.TempDir("", "lifecycle.test")
			if err != nil {
				t.Fatalf("Error: %s\n", err)
			}
			mkdir(t,
				filepath.Join(tmpDir, escapeID("buildpack/1"), "version1"),
				filepath.Join(tmpDir, "com.buildpack2", "version2.1"),
				filepath.Join(tmpDir, "com.buildpack2", "version2.2"),
				filepath.Join(tmpDir, "com.buildpack2", "version2.3"),
				filepath.Join(tmpDir, "com.buildpack2", "version2.4"),
				filepath.Join(tmpDir, "com.buildpack2", "latest"),
				filepath.Join(tmpDir, "buildpack-3", "version3"),
				filepath.Join(tmpDir, "buildpack4", "version4"),
			)
			mkBuildpackTOML(t, tmpDir, "buildpack/1", "buildpack1-name", "version1")
			mkBuildpackTOML(t, tmpDir, "com.buildpack2", "buildpack2-name", "version2.1")
			mkBuildpackTOML(t, tmpDir, "com.buildpack2", "buildpack2-name", "version2.2")
			mkVersionedBuildpackTOML(t, tmpDir, "com.buildpack2", "buildpack2-name", "version2.2", "latest")
			mkfile(t, "other",
				filepath.Join(tmpDir, "com.buildpack2", "version2.3", "not-buildpack.toml"),
				filepath.Join(tmpDir, "buildpack-3", "version3", "not-buildpack.toml"),
			)
			m, err := lifecycle.NewBuildpackMap(tmpDir)
			if s := cmp.Diff(m, lifecycle.BuildpackMap{
				"buildpack/1@version1": {
					ID:      "buildpack/1",
					Name:    "buildpack1-name",
					Version: "version1",
					Path:    filepath.Join(tmpDir, escapeID("buildpack/1"), "version1"),
				},
				"com.buildpack2@version2.1": {
					ID:      "com.buildpack2",
					Name:    "buildpack2-name",
					Version: "version2.1",
					Path:    filepath.Join(tmpDir, "com.buildpack2", "version2.1"),
				},
				"com.buildpack2@version2.2": {
					ID:      "com.buildpack2",
					Name:    "buildpack2-name",
					Version: "version2.2",
					Path:    filepath.Join(tmpDir, "com.buildpack2", "version2.2"),
				},
				"com.buildpack2@latest": {
					ID:      "com.buildpack2",
					Name:    "buildpack2-name",
					Version: "version2.2",
					Path:    filepath.Join(tmpDir, "com.buildpack2", "latest"),
				},
			}); s != "" {
				t.Fatalf("Unexpected map:\n%s\n", s)
			}
		})
	})

	when("#ReadOrder", func() {
		var tmpDir string

		it.Before(func() {
			var err error
			tmpDir, err = ioutil.TempDir("", "lifecycle.test")
			if err != nil {
				t.Fatal(err)
			}
		})

		it.After(func() {
			os.RemoveAll(tmpDir)
		})

		it("should return an ordering of buildpacks", func() {
			m := lifecycle.BuildpackMap{
				"buildpack1@version1.1": {Name: "buildpack1-1.1"},
				"buildpack1@version1.2": {Name: "buildpack1-1.2"},
				"buildpack2@latest":     {Name: "buildpack2"},
			}
			mkfile(t, `groups = [{ buildpacks = [{id = "buildpack1", version = "version1.1"}, {id = "buildpack2", optional = true}] }]`,
				filepath.Join(tmpDir, "order.toml"),
			)
			actual, err := m.ReadOrder(filepath.Join(tmpDir, "order.toml"))
			if err != nil {
				t.Fatal(err)
			}
			if s := cmp.Diff(actual, lifecycle.BuildpackOrder{
				{Group: []*lifecycle.Buildpack{{Name: "buildpack1-1.1"}, {Name: "buildpack2", Optional: true}}},
			}); s != "" {
				t.Fatalf("Unexpected list:\n%s\n", s)
			}
		})

		when("order references a missing buildpack", func() {
			it("returns an error", func() {
				m := lifecycle.BuildpackMap{
					"buildpack1@version1.2": {Name: "buildpack1-1.2"},
					"buildpack2@latest":     {Name: "buildpack2"},
				}
				mkfile(t, `groups = [{ buildpacks = [{id = "buildpack1", version = "version1.1"}, {id = "buildpack2", optional = true}] }]`,
					filepath.Join(tmpDir, "order.toml"),
				)
				_, err := m.ReadOrder(filepath.Join(tmpDir, "order.toml"))
				if err == nil {
					t.Fatal("expected an error")
				}
			})
		})
	})

	when("#ReadGroup", func() {
		var tmpDir string

		it.Before(func() {
			var err error
			tmpDir, err = ioutil.TempDir("", "lifecycle.test")
			if err != nil {
				t.Fatal(err)
			}
		})

		it.After(func() {
			os.RemoveAll(tmpDir)
		})

		it("should return a group of buildpacks", func() {
			m := lifecycle.BuildpackMap{
				"buildpack1@version1.1": {Name: "buildpack1-1.1"},
				"buildpack1@version1.2": {Name: "buildpack1-1.2"},
				"buildpack2@latest":     {Name: "buildpack2"},
			}
			mkfile(t, `buildpacks = [{id = "buildpack1", version = "version1.1"}, {id = "buildpack2", optional = true}]`,
				filepath.Join(tmpDir, "group.toml"),
			)
			actual, err := m.ReadGroup(filepath.Join(tmpDir, "group.toml"))
			if err != nil {
				t.Fatal(err)
			}
			if s := cmp.Diff(actual, &lifecycle.BuildpackGroup{
				Group: []*lifecycle.Buildpack{{Name: "buildpack1-1.1"}, {Name: "buildpack2", Optional: true}},
			}); s != "" {
				t.Fatalf("Unexpected list:\n%s\n", s)
			}
		})

		when("group references a missing buildpack", func() {
			it("returns an error", func() {
				m := lifecycle.BuildpackMap{
					"buildpack1@version1.2": {Name: "buildpack1-1.2"},
					"buildpack2@latest":     {Name: "buildpack2"},
				}
				mkfile(t, `buildpacks = [{id = "buildpack1", version = "version1.1"}, {id = "buildpack2", optional = true}]`,
					filepath.Join(tmpDir, "group.toml"),
				)
				_, err := m.ReadGroup(filepath.Join(tmpDir, "group.toml"))
				if err == nil {
					t.Fatal("expected an error")
				}
			})
		})
	})

	when("#Write", func() {
		var tmpDir string

		it.Before(func() {
			var err error
			tmpDir, err = ioutil.TempDir("", "lifecycle.test")
			if err != nil {
				t.Fatal(err)
			}
		})

		it.After(func() {
			os.RemoveAll(tmpDir)
		})

		it("should write only ID and version", func() {
			group := lifecycle.BuildpackGroup{
				Group: []*lifecycle.Buildpack{{ID: "a", Name: "b", Version: "v", Path: "d"}},
			}
			if err := group.Write(filepath.Join(tmpDir, "group.toml")); err != nil {
				t.Fatal(err)
			}
			b, err := ioutil.ReadFile(filepath.Join(tmpDir, "group.toml"))
			if err != nil {
				t.Fatal(err)
			}
			if s := cmp.Diff(string(b), "[[buildpacks]]\n  id = \"a\"\n  version = \"v\"\n"); s != "" {
				t.Fatalf(`toml did not match: (-got +want)\n%s`, s)
			}
		})
	})
}

const buildpackTOML = `
[buildpack]
id = "%[1]s"
name = "%[2]s"
version = "%[3]s"
dir = "none"
`

func mkBuildpackTOML(t *testing.T, dir, id, name, version string) {
	mkVersionedBuildpackTOML(t, dir, id, name, version, version)
}

func mkVersionedBuildpackTOML(t *testing.T, dir, id, name, version, dirname string) {
	mkfile(t, fmt.Sprintf(buildpackTOML, id, name, version),
		filepath.Join(dir, escapeID(id), dirname, "buildpack.toml"),
	)
}

func escapeID(id string) string {
	return strings.Replace(id, "/", string("___"), -1)
}
