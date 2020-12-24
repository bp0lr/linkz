package fileutils

import (
	"os"
	"path/filepath"
	"io/ioutil"
)

//GetFileNameFromLink desc
func GetFileNameFromLink(fullFilePath string) string{
	return filepath.Base(fullFilePath)
}

// CreateAndSaveToFile desc
func CreateAndSaveToFile(fullFilePath string, content []byte) (error){
	if _, err := MakeFilePath(filepath.Dir(fullFilePath), filepath.Base(fullFilePath)); err != nil {
		return err
	} 
	
	return ioutil.WriteFile(fullFilePath, content, 0664)
}

// CreateFile desc
func CreateFile(filePath string) (*os.File, error) {
	
	path, err := MakeFilePath(filepath.Dir(filePath), filepath.Base(filePath))
	if  err != nil {
		return nil, err
	} 
	
	return os.Create(path)	
}

//MakeFilePath desc
func MakeFilePath(dirName, fileName string) (string, error) {
	if err := EnsureDir(dirName); err != nil {
		return "", err
	}
	return filepath.Join(dirName, fileName), nil
}

//EnsureDir desc
func EnsureDir(dirName string, mode ...os.FileMode) error {
	m := os.FileMode(0750)
	if len(mode) > 0 {
		m = mode[0]
	}

	err := os.MkdirAll(dirName, m); 
	if err == nil || os.IsExist(err) {
		return nil
	}
	
	return err	
}

//FileExists desc
func FileExists(path string) bool {
	// os.Stat获取文件信息
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return false
	}
	return true
}