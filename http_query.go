package main

import (
	"net/http"

	"github.com/AudioAddict/go-echoprint/echoprint"
)

func queryHandler(w http.ResponseWriter, r *http.Request) {
	var jsonData []byte
	r.Body.Read(jsonData)

	matches, err := peformQuery(jsonData)
	if err != nil {
		apiError(w, err)
		return
	}
	renderResponse(w, matches)
}

func peformQuery(jsonData []byte) ([]interface{}, error) {
	codegenList, err := echoprint.ParseCodegen(jsonData)
	if err != nil {
		return nil, err
	}

	matches, err := echoprint.MatchAll(codegenList)
	return matches, err
}
