package bootstrapinplace

import (
	"encoding/base64"
	"encoding/json"
	"github.com/openshift/cluster-bootstrap/pkg/common"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	ignition "github.com/coreos/ignition/v2/config/v3_1"
	ignitionTypes "github.com/coreos/ignition/v2/config/v3_1/types"
)

type ConfigBootstrapInPlace struct {
	AssetDir     string
	IgnitionPath string
}

type BootstrapInPlaceCommand struct {
	ignitionPath string
	assetDir     string
}

func NewBootstrapInPlaceCommand(config ConfigBootstrapInPlace) (*BootstrapInPlaceCommand, error) {
	return &BootstrapInPlaceCommand{
		assetDir:     config.AssetDir,
		ignitionPath: config.IgnitionPath,
	}, nil
}

const (
	kubeDir                     = "/etc/kubernetes"
	assetPathBootstrapManifests = "bootstrap-manifests"
	manifests                   = "manifests"
	bootstrapConfigs            = "bootstrap-configs"
	bootstrapSecrets            = "bootstrap-secrets"
	etcdDataDir                 = "/var/lib/etcd"
	binDir                      = "/usr/local/bin"
)

type ignitionFile struct {
	filePath     string
	fileContents string
	mode         int
}

type filesToGather struct {
	pathForSearch string
	pattern       string
	ignitionPath  string
}

func newIgnitionFile(path string, filePathInIgnition string) (*ignitionFile, error) {
	content, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	encodedContent := "data:text/plain;charset=utf-8;base64," + base64.StdEncoding.EncodeToString(content)
	parentDir := filepath.Base(filepath.Dir(filePathInIgnition))
	var mode int
	if parentDir == "bin" {
		mode = 0555
	} else {
		mode = 0600
	}
	return &ignitionFile{filePath: filePathInIgnition,
		mode: mode, fileContents: encodedContent}, nil
}

func newIgnitionSystemdUnit(path string, enabled bool) (*ignitionTypes.Unit, error) {
	contents, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	name := filepath.Base(path)
	strContent := string(contents)
	return &ignitionTypes.Unit{
		Name:     name,
		Contents: &strContent,
		Enabled:  &enabled,
	}, nil
}

func (i *BootstrapInPlaceCommand) createListOfIgnitionFiles(files []string, searchedFolder string, folderInIgnition string) ([]*ignitionFile, error) {
	var ignitionFiles []*ignitionFile
	for _, path := range files {
		// Take relative path
		filePath := filepath.Join(folderInIgnition, strings.ReplaceAll(path, searchedFolder, ""))
		fileToAdd, err := newIgnitionFile(path, filePath)
		if err != nil {
			common.UserOutput("Failed to read %s\n", path)
			return nil, err
		}
		ignitionFiles = append(ignitionFiles, fileToAdd)
	}
	return ignitionFiles, nil
}

func (i *BootstrapInPlaceCommand) createFilesList(filesToGatherList []filesToGather) ([]*ignitionFile, error) {
	var fullList []*ignitionFile
	for _, ft := range filesToGatherList {
		files, err := i.findFiles(ft.pathForSearch, ft.pattern)
		if err != nil {
			common.UserOutput("Failed to search for files in %s with pattern %s, err %e\n", ft.pathForSearch, ft.pattern, err)
			return nil, err
		}
		ignitionFiles, err := i.createListOfIgnitionFiles(files, ft.pathForSearch, ft.ignitionPath)
		if err != nil {
			common.UserOutput("Failed to create ignitionsFile list for in %s with ign path %s, err %e\n", ft.pathForSearch, ft.ignitionPath, err)
			return nil, err
		}
		fullList = append(fullList, ignitionFiles...)
	}
	return fullList, nil
}

func (i *BootstrapInPlaceCommand) UpdateIgnitionWithBootstrapInPlaceData() error {

	common.UserOutput("Creating ignition file objects from required folders\n")
	filesFromFolders := []filesToGather{
		{pathForSearch: filepath.Join(i.assetDir, assetPathBootstrapManifests), pattern: "kube*", ignitionPath: filepath.Join(kubeDir, manifests)},
		{pathForSearch: filepath.Join(kubeDir, bootstrapConfigs), pattern: "*", ignitionPath: filepath.Join(kubeDir, bootstrapConfigs)},
		{pathForSearch: filepath.Join(i.assetDir, "tls"), pattern: "*", ignitionPath: filepath.Join(kubeDir, bootstrapSecrets)},
		{pathForSearch: filepath.Join(i.assetDir, "etcd-bootstrap/bootstrap-manifests/secrets"), pattern: "*", ignitionPath: filepath.Join(kubeDir, "static-pod-resources/etcd-member")},
		{pathForSearch: filepath.Join(i.assetDir, "bootstrap-in-place"), pattern: "bootstrap-in-place-post-reboot.sh", ignitionPath: filepath.Join(binDir)},
		{pathForSearch: etcdDataDir, pattern: "*", ignitionPath: etcdDataDir},
	}

	ignitionFileObjects, err := i.createFilesList(filesFromFolders)
	if err != nil {
		return err
	}

	common.UserOutput("Creating ignition file objects from files that require rename\n")
	singleFilesWithNameChange := map[string]string{
		filepath.Join(i.assetDir, "auth/kubeconfig-loopback"):                                filepath.Join(kubeDir, bootstrapSecrets+"/kubeconfig"),
		filepath.Join(i.assetDir, "tls/etcd-ca-bundle.crt"):                                  filepath.Join(kubeDir, "static-pod-resources/etcd-member/ca.crt"),
		filepath.Join(i.assetDir, "etcd-bootstrap/bootstrap-manifests/etcd-member-pod.yaml"): filepath.Join(kubeDir, manifests+"/etcd-pod.yaml"),
	}

	for path, ignPath := range singleFilesWithNameChange {
		fileToAdd, err := newIgnitionFile(path, ignPath)
		if err != nil {
			common.UserOutput("Error occurred while trying to create ignitionFile from %s with ign path %s, err : %e\n", path, ignPath, err)
			return err
		}
		ignitionFileObjects = append(ignitionFileObjects, fileToAdd)
	}
	ignitionUnitObjects := []*ignitionTypes.Unit{}
	for _, path := range []string{filepath.Join(i.assetDir,"bootstrap-in-place/bootstrap-in-place-post-reboot.service")} {
		unit, err := newIgnitionSystemdUnit(path, true)
		if err != nil {
			common.UserOutput("Failed to read system unit %s\n", path)
			return err
		}
		ignitionUnitObjects = append(ignitionUnitObjects, unit)
	}

	common.UserOutput("Ignition Path %s", i.ignitionPath)
	err = i.updateIgnitionFile(i.ignitionPath, ignitionFileObjects, ignitionUnitObjects)
	if err != nil {
		common.UserOutput("Error occurred while trying to read %s : %e\n", i.ignitionPath, err)
		return err
	}

	return nil
}

func (i *BootstrapInPlaceCommand) addFilesToIgnitionObject(ignitionData []byte, files []*ignitionFile) ([]byte, error) {

	ignitionOutput, _, err := ignition.Parse(ignitionData)
	if err != nil {
		return nil, err
	}

	for i := range files {
		common.UserOutput("Adding file %s\n", files[i].filePath)
		rootUser := "root"
		iFile := ignitionTypes.File{
			Node: ignitionTypes.Node{
				Path:      files[i].filePath,
				Overwrite: nil,
				Group:     ignitionTypes.NodeGroup{},
				User:      ignitionTypes.NodeUser{Name: &rootUser},
			},
			FileEmbedded1: ignitionTypes.FileEmbedded1{
				Append: []ignitionTypes.Resource{},
				Contents: ignitionTypes.Resource{
					Source: &files[i].fileContents,
				},
				Mode: &files[i].mode,
			},
		}
		ignitionOutput.Storage.Files = append(ignitionOutput.Storage.Files, iFile)
	}
	return json.Marshal(ignitionOutput)
}

func (i *BootstrapInPlaceCommand) updateIgnitionFile(ignitionPath string, files []*ignitionFile, units []*ignitionTypes.Unit) error {
	common.UserOutput("Adding files %d to ignition %s\n", len(files), ignitionPath)
	ignitionData, err := ioutil.ReadFile(ignitionPath)
	if err != nil {
		common.UserOutput("Error occurred while trying to read %s : %e\n", ignitionPath, err)
		return err
	}
	newIgnitionData, err := i.addFilesToIgnitionObject(ignitionData, files)
	if err != nil {
		common.UserOutput("Failed to write new ignition to %s : %e\n", ignitionPath, err)
		return err
	}

	newIgnitionData, err = i.addSystemdUnitsToIgnitionObject(newIgnitionData, units)

	err = ioutil.WriteFile(ignitionPath, newIgnitionData, os.ModePerm)
	if err != nil {
		common.UserOutput("Failed to write new ignition to %s\n", ignitionPath)
		return err
	}

	return nil
}

func (i *BootstrapInPlaceCommand) findFiles(root string, pattern string) ([]string, error) {
	var matches []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		if matched, err := filepath.Match(pattern, filepath.Base(path)); err != nil {
			return err
		} else if matched {
			matches = append(matches, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return matches, nil
}

func (i *BootstrapInPlaceCommand) addSystemdUnitsToIgnitionObject(ignitionData []byte, units []*ignitionTypes.Unit) ([]byte, error) {

	ignitionOutput, _, err := ignition.Parse(ignitionData)
	if err != nil {
		return nil, err
	}

	for _, unit := range units {
		common.UserOutput("Adding unit %s\n", unit.Name)
		ignitionOutput.Systemd.Units = append(ignitionOutput.Systemd.Units, *unit)
	}
	return json.Marshal(ignitionOutput)
}
