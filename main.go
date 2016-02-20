package main

import (
	"bufio"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	fp "path/filepath"
	"regexp"
	"strings"
)

func getOriginalFilePath(path string) (string, error) {
	fi, err := os.Lstat(path)
	if err != nil {
		return "", err
	}

	mode := fi.Mode()
	if mode&os.ModeSymlink != 0 {
		original, err := os.Readlink(path)
		if err != nil {
			return "", err
		}
		originalPath := fp.Join(fp.Dir(path), original)
		return getOriginalFilePath(originalPath)
	} else if mode.IsRegular() {
		return path, nil
	} else {
		return "", errors.New("Not a regular file or symlink")
	}
}

type Dep struct {
	Path       string // dylib path from otool
	RealPath   string // resolved path
	TargetPath string // the dest path
	FixPath    string // the new dylib path
}

type ObjectMap struct {
	TargetFolder string
	FixBaseDir   string
	ExecPath     string
	Deps         map[string][]Dep
}

func (om *ObjectMap) NewExec(filepath string) error {
	om.ExecPath = filepath
	return om.NewObject(filepath)
}

func (om *ObjectMap) NewObject(filepath string) error {
	// Key is in normalized form.
	normalized, err := getOriginalFilePath(filepath)
	if err != nil {
		return err
	}

	// Read its deps and set
	deps, err := om.ReadDeps(filepath)
	if err != nil {
		return err
	}
	om.Deps[normalized] = deps

	// Add new object from its deps only if it's a new dep
	for _, dep := range deps {
		if _, ok := om.Deps[dep.RealPath]; !ok {
			//log.Println(dep.Path, dep.RealPath, dep.TargetPath)
			om.NewObject(dep.RealPath)
		}
	}

	return nil
}

func (om *ObjectMap) ReadDeps(filepath string) ([]Dep, error) {
	cmd := exec.Command("otool", "-L", filepath)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	regex := regexp.MustCompile("^\t(.*) \\(.*\\)$")

	output := make([]Dep, 0)
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "\t") {

			path := regex.FindStringSubmatch(line)[1]
			normalized, err := getOriginalFilePath(path)
			if err != nil {
				return nil, err
			}

			// Exclude system library and frameworks
			if !strings.HasPrefix(normalized, "/usr/lib") &&
				strings.HasSuffix(normalized, ".dylib") {
				targetPath := fp.Join(om.TargetFolder, fp.Base(normalized))
				fixPath := om.FixBaseDir + fp.Base(normalized)

				dep := Dep{
					Path:       path,
					RealPath:   normalized,
					TargetPath: targetPath,
					FixPath:    fixPath,
				}
				output = append(output, dep)
			}

		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return output, nil
}

func (om ObjectMap) CopyToTargets() error {
	copied := make(map[string]bool)
	for _, deps := range om.Deps {
		for _, dep := range deps {
			if _, ok := copied[dep.TargetPath]; !ok {

				err := exec.Command("cp", "-rf", dep.RealPath, dep.TargetPath).Run()
				if err != nil {
					return err
				}

				err = os.Chmod(dep.TargetPath, 0644)
				if err != nil {
					return err
				}

				err = exec.Command("install_name_tool", "-id", dep.FixPath, dep.TargetPath).Run()
				if err != nil {
					return err
				}

				log.Println(dep.RealPath, " -> ", dep.TargetPath)
				copied[dep.TargetPath] = true
			}
		}
	}

	log.Printf("Copied %d dylibs.", len(copied))

	return nil
}

func (om ObjectMap) FixTargets() error {
	fixed := make(map[string]bool)
	for path, deps := range om.Deps {
		var targetPath string

		if om.ExecPath == path {
			targetPath = path
		} else {
			// Find target path
			normalized, err := getOriginalFilePath(path)
			if err != nil {
				return err
			}
			targetPath = fp.Join(om.TargetFolder, fp.Base(normalized))
		}

		if _, ok := fixed[targetPath]; !ok {
			log.Println("--> ", targetPath)
			for _, dep := range deps {
				err := exec.Command("install_name_tool", "-change", dep.Path, dep.FixPath, targetPath).Run()
				if err != nil {
					return err
				}
			}
			fixed[targetPath] = true
		}
	}

	log.Printf("Fixed %d dylibs.", len(fixed))

	return nil
}

func (om ObjectMap) Print() {
	for k, _ := range om.Deps {
		log.Println(k)
	}
}

func main() {
	if len(os.Args) < 4 {
		return
	}

	execPath := os.Args[1]
	targetFolder := os.Args[2]
	fixBaseDir := os.Args[3]

	var err error

	fmt.Println("Exec path:", execPath)
	fmt.Println("Target lib folder:", targetFolder)
	fmt.Println("Fix lib base folder:", fixBaseDir)

	objs := ObjectMap{
		TargetFolder: targetFolder,
		FixBaseDir:   fixBaseDir,
		Deps:         make(map[string][]Dep),
	}
	err = objs.NewExec(execPath)
	if err != nil {
		log.Fatal(err)
	}

	// Copy all deps to target directory
	if err = objs.CopyToTargets(); err != nil {
		log.Fatal(err)
	}

	// Fix the library path
	if err = objs.FixTargets(); err != nil {
		log.Fatal(err)
	}

	objs.Print()

	log.Printf("All done. %d dylibs are used.", len(objs.Deps))
}
