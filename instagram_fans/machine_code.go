package instagram_fans

import (
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"io/ioutil"
	"os"
)

func GetOrGenerateUUID(path string) (string, error) {
	if _, err := os.Stat(path); err == nil {
		data, err := ioutil.ReadFile(path)
		if err != nil {
			return "", errors.Wrap(err, "failed to read file")
		}
		return string(data), nil
	} else if os.IsNotExist(err) {
		newUUID := uuid.New().String()
		err := ioutil.WriteFile(path, []byte(newUUID), 0644)
		if err != nil {
			return "", errors.Wrap(err, "failed to write file")
		}
		return newUUID, nil
	} else {
		return "", errors.Wrap(err, "failed to get file info")
	}
}
