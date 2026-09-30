# remapper

Usage:

    remapper <tile width> <tile height> <atlas png> <map file>   classic: internal_name / name / icon records
    remapper <typed rec file>                                    a Tileset record + one section per category

Example: remapper 16 16 atlas.png map.rec

Keys:

F1  - Show hotkeys
s   - Save Changes
F10 - Quit
c   - Cut mode: arrows move the offset, shift+arrows the tile size, alt+arrows the gap
+/- - Zoom the atlas (mouse wheel over the atlas scrolls it)
Drop a png on the window - use it as the tileset (asks whether to keep the icon indexes)

Typed rec files only:

Tab - Next category (list filter)
F2  - Scene preview on/off; Space: next scene; click a cell to select its entry
Del - Mark the selected entry unassigned (icon: -1, drawn red)

## Typed rec files

The format is in `project.go` and in `~/Projects/c-rec/README.md` (the C reader
games use at runtime). Dropping a sheet in typed mode writes `<prefix>-<sheet>.rec`
(and copies the sheet) next to the current file, leaving the current file as it is.
Scenes (`%rec: Scene`: id, name, category, legend "<char> <category>/<id>", map,
and ground: the tile drawn under each map cell, as the game layers them) live in their own rec file, named by the Tileset's `scenes:` field.
Example: `~/Games/mag/port/mag-dawnlike.rec` and `mag-scenes.rec`.
