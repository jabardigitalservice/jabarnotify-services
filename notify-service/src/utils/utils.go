package utils

import (
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/360EntSecGroup-Skylar/excelize/v2"
	"github.com/joho/godotenv"
)

const (
	// DefaultLimit defines the default number of items per page for API responses
	DefaultLimit int = 25
)

//GetEnv func
func GetEnv(key string) string {
	// load .env file
	switch godotenv.Load() {
	case godotenv.Load("../.env"):
		log.Println("Error loading .env file")
	}
	return os.Getenv(key)
}

const charset = "abcdefghijklmnopqrstuvwxyz" +
	"ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

var seededRand *rand.Rand = rand.New(
	rand.NewSource(time.Now().UnixNano()))

// StringWithCharset func
func StringWithCharset(length int, charset string) string {
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[seededRand.Intn(len(charset))]
	}
	return string(b)
}

// String func
func String(length int) string {
	return StringWithCharset(length, charset)
}

// CleanPhoneNumber func
func CleanPhoneNumber(phoneNumber string) string {
	re := regexp.MustCompile("^0{1}")

	return re.ReplaceAllString(phoneNumber, "62")
}

// ReplaceBodyParams func
func ReplaceBodyParams(body string, params []string) string {

	for i := 0; i < len(params); i++ {
		param, value := fmt.Sprintf("{param%v}", i+1), params[i]
		body = strings.ReplaceAll(body, param, value)
	}

	return body
}

// ExtractSheet func
func ExtractSheet(r *http.Request, requestFile string) ([][]string, error) {
	r.ParseMultipartForm(10 << 20)

	file, handler, err := r.FormFile(requestFile)
	RandomString := String(30)
	exstension := filepath.Ext(handler.Filename)
	filename := RandomString + exstension
	defer file.Close()

	filepath := "./" + filename

	f, err := os.OpenFile(filepath, os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		fmt.Println(err)
		return nil, err
	}
	defer f.Close()
	io.Copy(f, file)

	//
	xlsFile, err := excelize.OpenFile(filepath)
	if err != nil {
		fmt.Println(err)
		return nil, err
	}

	// Get all the rows in the Sheet1.
	rows, err := xlsFile.GetRows("Sheet1")
	if err != nil {
		fmt.Println(err)
		return nil, err
	}

	os.Remove(filepath)

	return rows, nil
}

//PageCount func
func PageCount(total int, limit int) int {
	if limit == 0 {
		limit = DefaultLimit
	}
	pages := total / limit

	if total%limit > 0 {
		pages++
	}

	return pages
}
