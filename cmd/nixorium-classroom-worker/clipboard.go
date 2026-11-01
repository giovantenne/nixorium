package main

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/giovantenne/nixorium/internal/app"
	"github.com/giovantenne/nixorium/internal/classroomview"
)

// clipboard serves the shared clipboard of a computer under control. The text
// goes through here and is never logged or stored.
func (server *viewServer) clipboard(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		http.Error(writer, "Not available.", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Computer string `json:"computer"`
		Text     string `json:"text"`
		On       bool   `json:"on"`
	}
	// The text may be up to 64 KiB of UTF-8; JSON escapes can make it longer.
	decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, 4*classroomview.MaxClipboardBytes+1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil || !computerNamePattern.MatchString(body.Computer) {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			http.Error(writer, clipboardTooLarge, http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(writer, "Unknown computer.", http.StatusBadRequest)
		return
	}
	var err error
	switch request.URL.Path {
	case "/api/control":
		err = server.source.Control(body.Computer, body.On)
	case "/api/clipboard/set":
		var text string
		if text, err = classroomview.ValidateClipboardText(body.Text); err == nil {
			err = server.source.ClipboardSet(body.Computer, text)
		}
	default:
		var text string
		if text, err = server.source.ClipboardGet(body.Computer); err == nil {
			writer.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(writer).Encode(map[string]string{"text": text})
			return
		}
	}
	if err != nil {
		status, message := clipboardFailure(err)
		http.Error(writer, message, status)
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

const clipboardTooLarge = "That text is too long to share (the limit is 64 KiB)."

// clipboardFailure turns an error into a short plain message for the teacher;
// raw errors never reach the page.
func clipboardFailure(err error) (int, string) {
	var agent classroomview.AgentError
	switch {
	case errors.Is(err, classroomview.ErrClipboardTooLarge):
		return http.StatusRequestEntityTooLarge, clipboardTooLarge
	case errors.Is(err, app.ErrClassroomNotControlled), errors.Is(err, app.ErrClassroomNotViewed):
		return http.StatusConflict, "Take control of this computer first."
	case errors.Is(err, app.ErrClassroomLockedScreen):
		return http.StatusLocked, "This computer is locked."
	case errors.As(err, &agent) && agent.Code == classroomview.CodeLocked:
		return http.StatusLocked, "This computer is locked."
	case errors.As(err, &agent) && agent.Code == classroomview.CodeClipboardTooLarge:
		return http.StatusRequestEntityTooLarge, clipboardTooLarge
	}
	return http.StatusBadGateway, "The clipboard could not be shared with this computer."
}
