// Collect complete license texts from the pinned Go module graph and HEV tree.
// This is an inventory, not a replacement for reviewing distribution obligations.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

type module struct {
	Path, Version, Dir string
	Main               bool
}

func main() {
	if len(os.Args) != 2 {
		panic("project root argument required")
	}
	root, err := filepath.Abs(os.Args[1])
	must(err)
	cmd := exec.Command("go", "list", "-deps", "-json", ".")
	cmd.Dir = filepath.Join(root, "Engine")
	raw, err := cmd.Output()
	must(err)
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	var out strings.Builder
	out.WriteString("NaiLu iOS — third-party license inventory\n\nHEV pinned source: https://github.com/heiher/hev-socks5-tunnel/tree/0166c4765e9e42eee7d18923026f88fda91bd9fa\n\n")
	var missing []string
	modules := map[string]module{}
	for {
		var p struct{ Module *module }
		err := dec.Decode(&p)
		if err == io.EOF {
			break
		}
		must(err)
		if p.Module == nil || p.Module.Main {
			continue
		}
		modules[p.Module.Path] = *p.Module
	}
	// Bindings also link the gomobile runtime, which is not imported by the
	// wrapper package until gomobile generates its temporary main package.
	mobile := exec.Command("go", "list", "-m", "-json", "golang.org/x/mobile")
	mobile.Dir = cmd.Dir
	b, err := mobile.Output()
	must(err)
	var mobileModule module
	must(json.Unmarshal(b, &mobileModule))
	modules[mobileModule.Path] = mobileModule
	var paths []string
	for path := range modules {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		m := modules[path]
		if !appendLicenses(&out, m.Path+" "+m.Version, m.Dir) {
			missing = append(missing, m.Path)
		}
	}
	if !appendLicenses(&out, "Go standard library", runtime.GOROOT()) {
		missing = append(missing, "Go standard library")
	}
	// Git submodules have independent notices. Walk their source trees, excluding
	// generated build output and Git metadata; don't include configuration files.
	tree := filepath.Join(root, ".build", "hev-iphoneos-arm64")
	if source := os.Getenv("NAILU_HEV_SOURCE"); source != "" {
		tree = source
	}
	if _, err := os.Stat(tree); err == nil {
		must(filepath.WalkDir(tree, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if d.Name() == ".git" || d.Name() == "build" || d.Name() == "bin" {
					return filepath.SkipDir
				}
				return nil
			}
			if licenseName(d.Name()) {
				b, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				rel, _ := filepath.Rel(tree, path)
				fmt.Fprintf(&out, "\n========== HEV/%s ==========\n%s\n", rel, b)
			}
			return nil
		}))
	} else {
		missing = append(missing, "HEV source tree (run scripts/build-macos.sh)")
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		fmt.Fprintf(&out, "\nREQUIRES MANUAL LICENSE REVIEW (no root license found):\n%s\n", strings.Join(missing, "\n"))
	}
	must(os.WriteFile(filepath.Join(root, "Resources", "ThirdPartyLicenses.txt"), []byte(out.String()), 0644))
	fmt.Printf("License inventory generated; %d module roots have missing license files. License compatibility is not assessed by this inventory.\n", len(missing))
}
func licenseName(name string) bool {
	n := strings.ToUpper(name)
	return strings.HasPrefix(n, "LICENSE") || strings.HasPrefix(n, "COPYING") || strings.HasPrefix(n, "NOTICE")
}
func appendLicenses(out *strings.Builder, label, dir string) bool {
	if dir == "" {
		return false
	}
	files, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	found := false
	for _, f := range files {
		if !f.IsDir() && licenseName(f.Name()) {
			b, err := os.ReadFile(filepath.Join(dir, f.Name()))
			must(err)
			fmt.Fprintf(out, "\n========== %s / %s ==========\n%s\n", label, f.Name(), b)
			found = true
		}
	}
	return found
}
func must(err error) {
	if err != nil {
		panic(err)
	}
}

