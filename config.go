package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	//Path of the TOML files "https://github.com/BurntSushi/toml" in "C://Users/UName/git-mirror/src/"
	"github.com/BurntSushi/toml"
)

type duration struct {
	time.Duration
}

type config struct {
	ListenAddr string
	Interval   duration
	BasePath   string
	Repo       []repo
	Retries    string
	Counter    string
}

type repo struct {
	Name     string
	Origin   string
	Interval duration
}

//Service Declaration
type Service struct {
	Method  string
	Handler func(HandlerReq)
	RPC     string
}

// SmartConfig Declaration
type SmartConfig struct {
	AuthPassEnvVar string
	AuthUserEnvVar string
	DefaultEnv     string
	ProjectRoot    string
	GitBinPath     string
	UploadPack     bool
	ReceivePack    bool
}

//HandlerReq Declaration
type HandlerReq struct {
	w    http.ResponseWriter
	r    *http.Request
	RPC  string
	Dir  string
	File string
}

var smartconfig = SmartConfig{
	AuthPassEnvVar: "",
	AuthUserEnvVar: "",
	DefaultEnv:     "",
	ProjectRoot:    "/tmp",
	GitBinPath:     "/usr/bin/git",
	UploadPack:     true,
	ReceivePack:    true,
}

var services = map[string]Service{
	"(.*?)/git-upload-pack$":                       Service{"POST", serviceRPC, "upload-pack"},
	"(.*?)/git-receive-pack$":                      Service{"POST", serviceRPC, "receive-pack"},
	"(.*?)/info/refs$":                             Service{"GET", getInfoRefs, ""},
	"(.*?)/HEAD$":                                  Service{"GET", getTextFile, ""},
	"(.*?)/objects/info/alternates$":               Service{"GET", getTextFile, ""},
	"(.*?)/objects/info/http-alternates$":          Service{"GET", getTextFile, ""},
	"(.*?)/objects/info/packs$":                    Service{"GET", getInfoPacks, ""},
	"(.*?)/objects/info/[^/]*$":                    Service{"GET", getTextFile, ""},
	"(.*?)/objects/[0-9a-f]{2}/[0-9a-f]{38}$":      Service{"GET", getLooseObject, ""},
	"(.*?)/objects/pack/pack-[0-9a-f]{40}\\.pack$": Service{"GET", getPackFile, ""},
	"(.*?)/objects/pack/pack-[0-9a-f]{40}\\.idx$":  Service{"GET", getIdxFile, ""},
}

func (d *duration) UnmarshalText(text []byte) (err error) {
	d.Duration, err = time.ParseDuration(string(text))
	return
}

func parseConfig(filename string) (cfg config, repos map[string]repo, err error) {
	// Parse the raw TOML file.
	raw, err := ioutil.ReadFile(filename)
	if err != nil {
		err = fmt.Errorf("unable to read config file %s, %s", filename, err)
		return
	}
	if _, err = toml.Decode(string(raw), &cfg); err != nil {
		err = fmt.Errorf("unable to load config %s, %s", filename, err)
		return
	}

	// Set defaults if required.
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = ":8080"
	}
	if cfg.Interval.Duration == 0 {
		cfg.Interval.Duration = 15 * time.Minute
	}
	if cfg.BasePath == "" {
		cfg.BasePath = "."
	}
	smartconfig.ProjectRoot = cfg.BasePath
	if cfg.Retries == "" {
		cfg.Retries = "3"
	}
	if cfg.Counter == "" {
		cfg.Counter = "4"
	}
	if cfg.BasePath, err = filepath.Abs(cfg.BasePath); err != nil {
		err = fmt.Errorf("unable to get absolute path to base path, %s", err)
	}

	// Fetch repos, injecting default values where needed.
	if cfg.Repo == nil || len(cfg.Repo) == 0 {
		err = fmt.Errorf("no repos found in config %s, please define repos under [[repo]] sections", filename)
		return
	}
	repos = map[string]repo{}
	for i, r := range cfg.Repo {
		if r.Origin == "" {
			err = fmt.Errorf("Origin required for repo %d in config %s", i+1, filename)
			return
		}

		// Generate a name if there isn't one already
		if r.Name == "" {
			if u, err := url.Parse(r.Origin); err == nil && u.Scheme != "" {
				r.Name = u.Host + u.Path
			} else {
				parts := strings.Split(r.Origin, "@")
				if l := len(parts); l > 0 {
					r.Name = strings.Replace(parts[l-1], ":", "/", -1)
				}
			}
		}
		if r.Name == "" {
			err = fmt.Errorf("Could not generate name for Origin %s in config %s, please manually specify a Name", r.Origin, filename)
		}
		if _, ok := repos[r.Name]; ok {
			err = fmt.Errorf("Multiple repos with name %s in config %s", r.Name, filename)
			return
		}

		if r.Interval.Duration == 0 {
			r.Interval.Duration = cfg.Interval.Duration
		}
		repos[r.Name] = r
	}
	return
}
