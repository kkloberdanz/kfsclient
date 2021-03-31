/*
 *     Copyright (C) 2021 Kyle Kloberdanz
 *
 *  This program is free software: you can redistribute it and/or modify
 *  it under the terms of the GNU Affero General Public License as
 *  published by the Free Software Foundation, either version 3 of the
 *  License, or (at your option) any later version.
 *
 *  This program is distributed in the hope that it will be useful,
 *  but WITHOUT ANY WARRANTY; without even the implied warranty of
 *  MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 *  GNU Affero General Public License for more details.
 *
 *  You should have received a copy of the GNU Affero General Public License
 *  along with this program.  If not, see <https://www.gnu.org/licenses/>.
 */

package main

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func hash_file(filename string) (string, error) {
	output, err := exec.Command("b2sum", filename).Output()
	if err != nil {
		return "", fmt.Errorf("failed to hash '%s': %s", filename, err)
	}

	output_str := string(output)
	hash := strings.Fields(output_str)[0]
	return hash, nil
}

func upload(client *http.Client, url string, values map[string]io.Reader) (err error) {
	// Prepare a form that you will submit to that URL.
	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	for key, r := range values {
		var fw io.Writer
		if x, ok := r.(io.Closer); ok {
			defer x.Close()
		}
		// Add an image file
		if x, ok := r.(*os.File); ok {
			if fw, err = w.CreateFormFile(key, x.Name()); err != nil {
				return err
			}
		} else {
			// Add other fields
			if fw, err = w.CreateFormField(key); err != nil {
				return err
			}
		}
		if _, err = io.Copy(fw, r); err != nil {
			return err
		}

	}
	// Don't forget to close the multipart writer.
	// If you don't close it, your request will be missing the terminating boundary.
	w.Close()

	// Now that you have a form, you can submit it to your handler.
	req, err := http.NewRequest("POST", url, &b)
	if err != nil {
		return err
	}
	// Don't forget to set the content type, this will contain the boundary.
	req.Header.Set("Content-Type", w.FormDataContentType())

	// Submit the request
	res, err := client.Do(req)
	if err != nil {
		return err
	}

	buf := bytes.NewBuffer(make([]byte, 0, res.ContentLength))
	_, err = buf.ReadFrom(res.Body)
	if err != nil {
		return err
	}
	body := buf.Bytes()

	fmt.Printf("status: %s: %s\n", res.Status, body)
	// Check the response
	if res.StatusCode != http.StatusOK {
		err = fmt.Errorf("bad status: %s", res.Status)
	}
	return nil
}

func do_upload(server string, filename string) error {
	tr := &http.Transport{
		MaxIdleConns:       10,
		IdleConnTimeout:    30 * time.Second,
		DisableCompression: true,
	}
	client := &http.Client{Transport: tr}

	r, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer r.Close()

	hash, err := hash_file(filename)
	if err != nil {
		return err
	}

	values := map[string]io.Reader{
		"file": r,
		"hash": strings.NewReader(hash),
		"path": strings.NewReader(filename),
	}

	url := fmt.Sprintf("%s/upload", server)
	err = upload(client, url, values)
	return err
}

func check_exists(server string, filename string) (bool, error) {
	hash, err := hash_file(filename)
	if err != nil {
		return false, err
	}
	url := fmt.Sprintf("%s/exists/%s", server, hash)
	res, err := http.Get(url)
	if err != nil {
		return false, err
	}
	buf := bytes.NewBuffer(make([]byte, 0, res.ContentLength))
	_, err = buf.ReadFrom(res.Body)
	if err != nil {
		return false, err
	}
	body := buf.String()
	//fmt.Printf("body: %s\n", body)
	return body == "yes", nil
}

func main() {
	nargs := len(os.Args)
	if nargs != 3 {
		panic(fmt.Sprintf("usage: %s SERVER FILENAME", os.Args[0]))
	}
	server := os.Args[1]
	filename, err := filepath.Abs(os.Args[2])
	if err != nil {
		panic(err)
	}

	exists, err := check_exists(server, filename)
	if err != nil {
		panic(err)
	}

	if !exists {
		err := do_upload(server, filename)
		if err != nil {
			panic(err)
		}
	} else {
		fmt.Printf("file already exists: '%s'\n", filename)
	}
}
