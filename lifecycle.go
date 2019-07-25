package lifecycle

var POSIXBuildEnv = map[string][]string{
	"bin": {
		"PATH",
	},
	"lib": {
		"LD_LIBRARY_PATH",
		"LIBRARY_PATH",
	},
	"include": {
		"CPATH",
		"C_INCLUDE_PATH",
		"CPLUS_INCLUDE_PATH",
		"OBJC_INCLUDE_PATH",
	},
	"pkgconfig": {
		"PKG_CONFIG_PATH",
	},
}

var POSIXLaunchEnv = map[string][]string{
	"bin": {"PATH"},
	"lib": {"LD_LIBRARY_PATH"},
}

type buildpackTOML struct {
	Buildpack buildpackInfo  `toml:"buildpack"`
	Order     BuildpackOrder `toml:"order"`
	Path      string         `toml:"-"`
}

type buildpackInfo struct {
	ID      string `toml:"id"`
	Version string `toml:"version"`
	Name    string `toml:"name"`
}

func (bp buildpackTOML) String() string {
	return bp.Buildpack.Name + " " + bp.Buildpack.Version
}
