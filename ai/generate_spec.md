# Create a program in //cmd/generate

* Read the file //GEMINI.md for general rules and approaches.
* Use `bazel build //...` to verify if the project builds.
* For each task or subtask, ask user for confirmation if they want to create
  a pull request, and upon positive answer, create a pull request.

Then perform the task outlined below.

## Task

Generate a program that converts the contents of the directory //modules into
a single web page, with requirements below.

Program is written in go.

The page should iterate over all directories beneath `//modules`.

For each found directory, consult the bazel repository data schema, analyze
the files in the directory and generate a tabular entry for each directory,
with links to each of the modules, module versions, and which registers all the
metadata for each entry as well.

Use bootstrap to style the page.

Place the result into the file `//cmd/generate/main.go`.

## Subtask 1

Modify the code, such that the output file is taken as a flag `-input=...`,
instead of computed from `BUILD_WORKING_DIRECTORY`.

Modify the `go_binary` rule to load this file automatically through `args`.

## Subtask 2

Modify the HTML template in `//cmd/generate/main.go` as follows:

* Convert each row into a bootstrap card.
* Order all such card into a free flowing grid.
* Order all versions one after another, separated by a comma.
* Modify href for each version to point to the location of that version's
  metadata in the repository https://github.com/filmil/bazel-registry.

## Subtask 3

Modify the HTML template in `//cmd/generate/main.go` as follows:

* Add a link icon next to each module name that points to the respective module's
home page.
* Link each repository name (for example `github:filmil/bazel-bats` and similar)
  to its respective github repository URL.
* Rebuild the project to ensure that the change is correct.

## Subtask 4


Modify the HTML template in `//cmd/generate/main.go` as follows:

* At the beginning add a text input box. Use proper styling from bootstrap.
* Add javascripting to the page that will fuzzy-filter the list of module cards
  to show only those that match the fuzzy filter based on the text in the input
  box.
* Update the list of module cards on each change to the text input box.
* Ensure that when the text input box is empty, that all cards are shown.


## Subtask 5


Modify the HTML template in `//cmd/generate/main.go` as follows:

* For each module version, add a mouse-over which on hover displays the text
  needed to add this module into `MODULE.bazel` when used. Add a "copy" icon
  that on click copies that text into the current clipboard.
* Use double quotes for quoting strings in the mouse-over.


## Subtask 6


Modify the HTML template in `//cmd/generate/main.go` as follows:

* Add a page bottom matter that has a copyright to Filip Filmar from year 2025
  onwards, and add a note that the code to generate the page was created in
  full with an automated coding assistant.


## Subtask 7


Modify the HTML template in `//cmd/generate/main.go` as follows:

* If a module version appears in the respective module's metadata.json file
  in `yanked_versions`, then render it as strikethrough and do not add any
  links to the version text.

