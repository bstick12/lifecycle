package lifecycle_test

import (
	"bytes"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/lifecycle"
)

func TestDetector(t *testing.T) {
	spec.Run(t, "Detector", testDetector, spec.Report(report.Terminal{}))
}

func testDetector(t *testing.T, when spec.G, it spec.S) {
	var (
		config *lifecycle.DetectConfig
		outLog *bytes.Buffer
		tmpDir string
	)

	it.Before(func() {
		var err error
		tmpDir, err = ioutil.TempDir("", "lifecycle")
		if err != nil {
			t.Fatalf("Error: %s\n", err)
		}
		platformDir := filepath.Join(tmpDir, "platform")
		appDir := filepath.Join(tmpDir, "app")
		mkdir(t, appDir, filepath.Join(platformDir, "env"))

		buildpacksDir := filepath.Join("testdata", "by-id")

		outLog = &bytes.Buffer{}
		config = &lifecycle.DetectConfig{
			AppDir:        appDir,
			PlatformDir:   platformDir,
			BuildpacksDir: buildpacksDir,
			Out:           log.New(io.MultiWriter(outLog, it.Out()), "", 0),
		}
	})

	it.After(func() {
		os.RemoveAll(tmpDir)
	})

	mkappfile := func(data string, paths ...string) {
		t.Helper()
		for _, p := range paths {
			mkfile(t, data, filepath.Join(config.AppDir, p))
		}
	}
	toappfile := func(data string, paths ...string) {
		t.Helper()
		for _, p := range paths {
			tofile(t, data, filepath.Join(config.AppDir, p))
		}
	}
	rdappfile := func(path string) string {
		t.Helper()
		return rdfile(t, filepath.Join(config.AppDir, path))
	}

	when("#Detect", func() {
		it("should expand order-containing buildpack IDs", func() {
			mkappfile("100", "detect-status")

			_, _, err := lifecycle.BuildpackOrder{
				{Group: []lifecycle.Buildpack{{ID: "E", Version: "v1"}}},
			}.Detect(config)
			if err != lifecycle.ErrFail {
				t.Fatalf("Unexpected error:\n%s\n", err)
			}

			if s := cmp.Diff("\n"+outLog.String(), outputFailureEv1); s != "" {
				t.Fatalf("Unexpected log:\n%s\n", s)
			}
		})

		it("should select the first passing group", func() {
			mkappfile("100", "detect-status")
			mkappfile("0", "detect-status-A-v1", "detect-status-B-v1")

			group, plan, err := lifecycle.BuildpackOrder{
				{Group: []lifecycle.Buildpack{{ID: "E", Version: "v1"}}},
			}.Detect(config)
			if err != nil {
				t.Fatalf("Unexpected error:\n%s\n", err)
			}

			if s := cmp.Diff(group, lifecycle.BuildpackGroup{
				Group: []lifecycle.Buildpack{
					{ID: "A", Version: "v1"},
					{ID: "B", Version: "v1"},
				},
			}); s != "" {
				t.Fatalf("Unexpected group:\n%s\n", s)
			}

			if s := cmp.Diff(plan.Entries, []lifecycle.DetectPlanEntry(nil)); s != "" {
				t.Fatalf("Unexpected :\n%s\n", s)
			}

			if s := outLog.String(); !strings.HasSuffix(s,
				"======== Results ========\n"+
					"pass: A@v1\n"+
					"pass: B@v1\n",
			) {
				t.Fatalf("Unexpected results:\n%s\n", s)
			}

			bpDir, err := filepath.Abs(filepath.Join(config.BuildpacksDir, "A", "v1"))
			if err != nil {
				t.Fatalf("Unexpected error:\n%s\n", err)
			}

			if s := cmp.Diff(rdappfile("detect-info-A-v1"),
				"Path: "+bpDir+"\n"+
					"TOML: "+filepath.Join(bpDir, "buildpack.toml")+"\n",
			); s != "" {
				t.Fatalf("Unexpected :\n%s\n", s)
			}
		})

		it("should fail if the group is empty", func() {
			_, _, err := lifecycle.BuildpackOrder([]lifecycle.BuildpackGroup{{}}).Detect(config)
			if err != lifecycle.ErrFail {
				t.Fatalf("Unexpected error:\n%s\n", err)
			}

			if s := cmp.Diff(outLog.String(),
				"======== Results ========\n"+
					"fail: no viable buildpacks in group\n",
			); s != "" {
				t.Fatalf("Unexpected log:\n%s\n", s)
			}
		})

		it("should fail if the group has no viable buildpacks, even if no required buildpacks fail", func() {
			mkappfile("100", "detect-status")
			_, _, err := lifecycle.BuildpackOrder{
				{Group: []lifecycle.Buildpack{
					{ID: "A", Version: "v1", Optional: true},
					{ID: "B", Version: "v1", Optional: true},
				}},
			}.Detect(config)
			if err != lifecycle.ErrFail {
				t.Fatalf("Unexpected error:\n%s\n", err)
			}

			if s := outLog.String(); !strings.HasSuffix(s,
				"======== Results ========\n"+
					"skip: A@v1\n"+
					"skip: B@v1\n"+
					"fail: no viable buildpacks in group\n",
			) {
				t.Fatalf("Unexpected results:\n%s\n", s)
			}
		})

		when("a build plan is employed", func() {
			it("should return a build plan with matched dependencies", func() {
				mkappfile("100", "detect-status-C-v1")
				mkappfile("100", "detect-status-B-v2")

				toappfile("\n[[provides]]\n name = \"dep1\"", "detect-plan-A-v1.toml", "detect-plan-C-v2.toml")
				toappfile("\n[[provides]]\n name = \"dep2\"", "detect-plan-A-v1.toml", "detect-plan-C-v2.toml")
				toappfile("\n[[provides]]\n name = \"dep2\"", "detect-plan-D-v2.toml")

				toappfile("\n[[requires]]\n name = \"dep1\"", "detect-plan-D-v2.toml", "detect-plan-B-v1.toml")
				toappfile("\n[[requires]]\n name = \"dep2\"", "detect-plan-D-v2.toml", "detect-plan-B-v1.toml")
				toappfile("\n[[requires]]\n name = \"dep2\"", "detect-plan-A-v1.toml")

				group, plan, err := lifecycle.BuildpackOrder{
					{Group: []lifecycle.Buildpack{
						{ID: "A", Version: "v1"},
						{ID: "C", Version: "v2"},
						{ID: "D", Version: "v2"},
						{ID: "B", Version: "v1"},
					}},
				}.Detect(config)
				if err != nil {
					t.Fatalf("Unexpected error:\n%s\n", err)
				}

				if s := cmp.Diff(group, lifecycle.BuildpackGroup{
					Group: []lifecycle.Buildpack{
						{ID: "A", Version: "v1"},
						{ID: "C", Version: "v2"},
						{ID: "D", Version: "v2"},
						{ID: "B", Version: "v1"},
					},
				}); s != "" {
					t.Fatalf("Unexpected group:\n%s\n", s)
				}

				if s := cmp.Diff(plan.Entries, []lifecycle.DetectPlanEntry{
					{
						Providers: []lifecycle.Buildpack{
							{ID: "A", Version: "v1"},
							{ID: "C", Version: "v2"},
						},
						Requires: []lifecycle.Require{{Name: "dep1"}, {Name: "dep1"}},
					},
					{
						Providers: []lifecycle.Buildpack{
							{ID: "A", Version: "v1"},
							{ID: "C", Version: "v2"},
							{ID: "D", Version: "v2"},
						},
						Requires: []lifecycle.Require{{Name: "dep2"}, {Name: "dep2"}, {Name: "dep2"}},
					},
				}); s != "" {
					t.Fatalf("Unexpected :\n%s\n", s)
				}

				if s := outLog.String(); !strings.HasSuffix(s,
					"======== Results ========\n"+
						"pass: A@v1\n"+
						"pass: C@v2\n"+
						"pass: D@v2\n"+
						"pass: B@v1\n",
				) {
					t.Fatalf("Unexpected results:\n%s\n", s)
				}
			})

			it("should fail if all requires are not provided first", func() {
				toappfile("\n[[provides]]\n name = \"dep1\"", "detect-plan-A-v1.toml", "detect-plan-C-v1.toml")
				toappfile("\n[[requires]]\n name = \"dep1\"", "detect-plan-B-v1.toml", "detect-plan-C-v1.toml")
				mkappfile("100", "detect-status-A-v1")

				_, _, err := lifecycle.BuildpackOrder{
					{Group: []lifecycle.Buildpack{
						{ID: "A", Version: "v1", Optional: true},
						{ID: "B", Version: "v1"},
						{ID: "C", Version: "v1"},
					}},
				}.Detect(config)
				if err != lifecycle.ErrFail {
					t.Fatalf("Unexpected error:\n%s\n", err)
				}

				if s := outLog.String(); !strings.HasSuffix(s,
					"======== Results ========\n"+
						"skip: A@v1\n"+
						"pass: B@v1\n"+
						"pass: C@v1\n"+
						"fail: B@v1 requires dep1\n",
				) {
					t.Fatalf("Unexpected results:\n%s\n", s)
				}
			})

			it("should fail if all provides are not required after", func() {
				toappfile("\n[[provides]]\n name = \"dep1\"", "detect-plan-A-v1.toml", "detect-plan-B-v1.toml")
				toappfile("\n[[requires]]\n name = \"dep1\"", "detect-plan-A-v1.toml", "detect-plan-C-v1.toml")
				mkappfile("100", "detect-status-C-v1")

				_, _, err := lifecycle.BuildpackOrder{
					{Group: []lifecycle.Buildpack{
						{ID: "A", Version: "v1"},
						{ID: "B", Version: "v1"},
						{ID: "C", Version: "v1", Optional: true},
					}},
				}.Detect(config)
				if err != lifecycle.ErrFail {
					t.Fatalf("Unexpected error:\n%s\n", err)
				}

				if s := outLog.String(); !strings.HasSuffix(s,
					"======== Results ========\n"+
						"pass: A@v1\n"+
						"pass: B@v1\n"+
						"skip: C@v1\n"+
						"fail: B@v1 provides unused dep1\n",
				) {
					t.Fatalf("Unexpected results:\n%s\n", s)
				}
			})

			it("should succeed if unmet provides/requires are optional", func() {
				toappfile("\n[[requires]]\n name = \"dep-missing\"", "detect-plan-A-v1.toml")
				toappfile("\n[[provides]]\n name = \"dep-missing\"", "detect-plan-C-v1.toml")
				toappfile("\n[[requires]]\n name = \"dep-present\"", "detect-plan-B-v1.toml")
				toappfile("\n[[provides]]\n name = \"dep-present\"", "detect-plan-B-v1.toml")

				group, plan, err := lifecycle.BuildpackOrder{
					{Group: []lifecycle.Buildpack{
						{ID: "A", Version: "v1", Optional: true},
						{ID: "B", Version: "v1"},
						{ID: "C", Version: "v1", Optional: true},
					}},
				}.Detect(config)
				if err != nil {
					t.Fatalf("Unexpected error:\n%s\n", err)
				}

				if s := cmp.Diff(group, lifecycle.BuildpackGroup{
					Group: []lifecycle.Buildpack{
						{ID: "B", Version: "v1"},
					},
				}); s != "" {
					t.Fatalf("Unexpected group:\n%s\n", s)
				}

				if s := cmp.Diff(plan.Entries, []lifecycle.DetectPlanEntry{
					{
						Providers: []lifecycle.Buildpack{{ID: "B", Version: "v1"}},
						Requires:  []lifecycle.Require{{Name: "dep-present"}},
					},
				}); s != "" {
					t.Fatalf("Unexpected :\n%s\n", s)
				}

				if s := outLog.String(); !strings.HasSuffix(s,
					"======== Results ========\n"+
						"pass: A@v1\n"+
						"pass: B@v1\n"+
						"pass: C@v1\n"+
						"skip: A@v1 requires dep-missing\n"+
						"skip: C@v1 provides unused dep-missing\n",
				) {
					t.Fatalf("Unexpected results:\n%s\n", s)
				}
			})
		})
	})
}

var outputFailureEv1 = `
======== Output: A@v1 ========
detect out: A@v1
detect err: A@v1
======== Output: C@v1 ========
detect out: C@v1
detect err: C@v1
======== Output: B@v1 ========
detect out: B@v1
detect err: B@v1
======== Results ========
fail: A@v1
fail: C@v1
fail: B@v1
======== Output: A@v1 ========
detect out: A@v1
detect err: A@v1
======== Output: B@v2 ========
detect out: B@v2
detect err: B@v2
======== Results ========
fail: A@v1
fail: B@v2
======== Output: A@v1 ========
detect out: A@v1
detect err: A@v1
======== Output: C@v2 ========
detect out: C@v2
detect err: C@v2
======== Output: D@v2 ========
detect out: D@v2
detect err: D@v2
======== Output: B@v1 ========
detect out: B@v1
detect err: B@v1
======== Results ========
fail: A@v1
fail: C@v2
fail: D@v2
fail: B@v1
======== Output: A@v1 ========
detect out: A@v1
detect err: A@v1
======== Output: B@v1 ========
detect out: B@v1
detect err: B@v1
======== Results ========
fail: A@v1
fail: B@v1
======== Output: A@v1 ========
detect out: A@v1
detect err: A@v1
======== Output: D@v1 ========
detect out: D@v1
detect err: D@v1
======== Output: B@v1 ========
detect out: B@v1
detect err: B@v1
======== Results ========
fail: A@v1
fail: D@v1
fail: B@v1
`
