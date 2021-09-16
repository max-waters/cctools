package util

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

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

func FormatFileName(programName string) string {
	if extension := filepath.Ext(programName); len(extension) > 0 { // return eg file-timestamp.extension
		name := strings.TrimSuffix(programName, extension)
		return fmt.Sprintf("%s-%s%s", name, time.Now().Format(TimestampFormat), extension)
	}
	// return file-timestamp.csv
	return fmt.Sprintf("%s-%s.csv", programName, time.Now().Format(TimestampFormat))
}
