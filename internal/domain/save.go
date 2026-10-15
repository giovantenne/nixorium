package domain

// Saving files the classroom service holds (screenshots it took, desktops
// it collected) into the home of the user who asked: that user's own
// process copies them with save-items and save-read, and says so with
// save-done. Only the user the files were prepared for may read them.
// Requests name the save with DesktopJob, read with ShareIndex and
// ShareOffset and report the names saved with SaveNames; answers carry
// DesktopKind and one page of SaveItems (SaveTotal in all), or SaveData.
const (
	ClassroomSaveItemsOperation ClassroomOperation = "save-items"
	ClassroomSaveReadOperation  ClassroomOperation = "save-read"
	ClassroomSaveDoneOperation  ClassroomOperation = "save-done"
)

// saveOperations are the classroom operations of this file.
var saveOperations = map[ClassroomOperation]bool{
	ClassroomSaveItemsOperation: true,
	ClassroomSaveReadOperation:  true,
	ClassroomSaveDoneOperation:  true,
}

// Desktop helper jobs that save into the user's home: screenshots go to
// ~/Nixorium/Screenshots and collected desktops to ~/Nixorium/Collected.
const (
	DesktopSaveScreenshots = "save-screenshots"
	DesktopSaveCollected   = "save-collected"
)

// ClassroomSaveItemsPage bounds the items of one save-items answer, and
// ClassroomSaveNameBytes the bytes of their names, so that an answer stays
// well below the classroom message limit.
const (
	ClassroomSaveItemsPage = 1000
	ClassroomSaveNameBytes = 96 << 10
)
