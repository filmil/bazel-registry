# Create a program in //cmd/generate

Read the file //GEMINI.md for general rules and approaches.

Then perform the task outlined below.

## Task

Generate a program that converts the contents of the directory //modules into
a single web page, with requirements below.

The page should iterate over all directories beneath `//modules`.

For each found directory, consult the bazel repository data schema, analyze
the files in the directory and generate a tabular entry for each directory,
with links to each of the modules, module versions, and which registers all the
metadata for each entry as well.

Use bootstrap to style the page.

Place the result into the file `//cmd/generate/main.go`.
