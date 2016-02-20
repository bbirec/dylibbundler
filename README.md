dylibbundler
------------
A Go port of [macdylibbundler](https://github.com/auriamg/macdylibbundler).

## Usage
```
dylibbundler [executable path] [destination directory] [install path]
```

## Sample
Add a build phase with following shell script in XCode.
It copies all of dylibs to `libs` directory and fixes all of its install names.

```
mkdir -p $BUILT_PRODUCTS_DIR/$CONTENTS_FOLDER_PATH/libs/
dylibbundler $BUILT_PRODUCTS_DIR/$EXECUTABLE_PATH $BUILT_PRODUCTS_DIR/$CONTENTS_FOLDER_PATH/libs/ @executable_path/../libs/
```
