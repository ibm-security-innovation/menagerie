# Sample engine - apktool
A sample engine that wraps the [apktool](http://ibotpeaches.github.io/Apktool/)
Android reversing tool. This shows the usefulness of Menagerie as a platform for
installing file analyzers, without the need to install anything on the base
platform.

`./build.sh` builds, tags, and pushes to local docker registry

Container can be executed by placing a `sample.apk` in a `test/` directory and
  running `docker run --rm -v $PWD/test:/var/data/ localhost:5000/apktool:latest /engine/run.sh`

Note that the input file name is hard-coded to `sample.apk` - this is because
menagerie is copying the sample into the shared mount under this name as
configured in `../confs/engines.json`. The output file is always named `result`
to be collected by menagerie - in this case it is a ZIP file.
