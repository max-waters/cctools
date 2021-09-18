package util

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/gocarina/gocsv"
	"github.com/pkg/errors"
)

func LoadControllerValues(filename string) ([]*ControllerValue, error) {
	rows := []*ControllerValue{}
	if err := UnmarshalCsv(filename, &rows); err != nil {
		return nil, errors.Wrap(err, "cannot load controller values")
	}
	return rows, nil
}

func LoadVoiceControllerValues(filename string) ([]*VoiceControllerValue, error) {
	rows := []*VoiceControllerValue{}
	if err := UnmarshalCsv(filename, &rows); err != nil {
		return nil, errors.Wrap(err, "cannot load controller values")
	}
	return rows, nil
}

func UnmarshalCsv(filename string, dest interface{}) error {
	f, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer f.Close()
	return gocsv.Unmarshal(f, dest)
}

func SaveVoiceControllerValues(filename string, data []*VoiceControllerValue) (string, error) {
	filename, err := MarshalCsv(filename, &data)
	if err != nil {
		return "", errors.Wrap(err, "cannot save controller values")
	}
	return filename, nil
}

func SaveControllerValues(filename string, data []*ControllerValue) (string, error) {
	filename, err := MarshalCsv(filename, &data)
	if err != nil {
		return "", errors.Wrap(err, "cannot save controller values")
	}
	return filename, nil
}

func MarshalCsv(filename string, data interface{}) (string, error) {
	filename = FormatFileName(filename)
	if err := BackupIfExists(filename); err != nil {
		return "", errors.Wrap(err, "cannot back up file")
	}

	// create parent directories
	if err := os.MkdirAll(filepath.Dir(filename), os.ModePerm); err != nil {
		return "", err
	}

	// create file
	f, err := os.Create(filename)
	if err != nil {
		return "", err
	}
	defer f.Close()

	// write data
	return filename, gocsv.Marshal(data, f)
}

func BackupIfExists(filename string) error {
	// if it is a new file, return
	if _, err := os.Stat(filename); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		} else {
			return errors.Wrapf(err, "cannot check if file '%s' exists", filename)
		}
	}

	// find backup file name
	dir := filepath.Dir(filename)
	extension := filepath.Ext(filename)
	fileBase := strings.TrimSuffix(filepath.Base(filename), extension)

	regex, err := GetNumberedFileRegex(fileBase, extension)
	if err != nil {
		return err
	}

	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return errors.Wrapf(err, "cannot read contents of directory '%s'", dir)
	}

	highest := -1
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		matches := regex.FindStringSubmatch(file.Name())
		if len(matches) == 0 {
			continue
		}

		n, err := strconv.Atoi(matches[1])
		if err != nil {
			return err
		}
		if n > highest {
			highest = n
		}
	}
	backupFileName := fmt.Sprintf("%s/%s-%03d%s", dir, fileBase, highest+1, extension)

	// copy current file to backup
	source, err := os.Open(filename)
	if err != nil {
		return errors.Wrapf(err, "cannot open file '%s'", filename)
	}
	defer source.Close()

	destination, err := os.Create(backupFileName)
	if err != nil {
		return errors.Wrapf(err, "cannot open file '%s'", backupFileName)
	}
	defer destination.Close()

	if _, err := io.Copy(destination, source); err != nil {
		return errors.Wrap(err, "cannot copy to backup file")
	}

	return nil
}

func FormatFileName(filename string) string {
	if ext := filepath.Ext(filename); len(ext) > 0 { // file has an extension, keep it
		return filename
	}
	return filename + ".csv"
}

func GetNumberedFileRegex(filename, extension string) (*regexp.Regexp, error) {
	if len(extension) > 0 && extension[0] != '.' {
		extension = "." + extension
	}
	sanitisedFileName := regexp.QuoteMeta(filename)
	sanitisedExt := regexp.QuoteMeta(extension)
	return regexp.Compile(fmt.Sprintf("^%s-([\\d]+)%s$", sanitisedFileName, sanitisedExt))
}
