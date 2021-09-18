package util

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/gocarina/gocsv"
	"github.com/pkg/errors"
)

const TimestampFormat = "2006-01-02T15:04:05"

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
		return nil
	}
	defer f.Close()
	return gocsv.Unmarshal(f, dest)
}

func SaveVoiceControllerValues(filename string, data []*VoiceControllerValue) error {
	if err := MarshalCsv(filename, &data); err != nil {
		return errors.Wrap(err, "cannot save controller values")
	}
	return nil
}

func SaveControllerValues(filename string, data []*ControllerValue) error {
	if err := MarshalCsv(filename, &data); err != nil {
		return errors.Wrap(err, "cannot controller values")
	}
	return nil
}

func MarshalCsv(filename string, data interface{}) error {
	// create parent directories
	if err := os.MkdirAll(filepath.Dir(filename), os.ModePerm); err != nil {
		return err
	}
	// create file
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	// write data
	return gocsv.Marshal(data, f)
}

func FormatFileName(filename string) (string, error) {
	extension := ".csv"
	if ext := filepath.Ext(filename); len(ext) > 0 { // file has an extension
		extension = ext
	}
	dir := filepath.Dir(filename)
	filebase := strings.TrimSuffix(filepath.Base(filename), extension)

	n, err := GetNextFileNumber(dir, filebase, extension[1:])
	if err != nil {
		return "", errors.New("error formatting filename")
	}
	return fmt.Sprintf("%s/%s-%03d%s", dir, filebase, n, extension), nil
}

func GetNextFileNumber(dir, filename, extension string) (int, error) {
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		log.Fatal(err)
	}

	regex, err := GetNumberedFileRegex(filename, extension)
	if err != nil {
		return -1, err
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
			return -1, err
		}
		if n > highest {
			highest = n
		}
	}
	return highest + 1, nil
}

func GetNumberedFileRegex(filename, extension string) (*regexp.Regexp, error) {
	sanitisedFileName := regexp.QuoteMeta(filename)
	sanitisedExt := regexp.QuoteMeta(extension)
	return regexp.Compile(fmt.Sprintf("^%s-([\\d]+).%s$", sanitisedFileName, sanitisedExt))
}
