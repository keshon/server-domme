# Application launcher helpers

`ensure-build.cmd` and `ensure-build.sh` are shared by the root application
launchers. In a source checkout they rebuild the requested application; in an
extracted distribution they use the supplied executable. Sharing this logic
keeps the launchers small and gives both shells the same release fallback.
The directory is functional, not required just to match another repository.

`build.cmd` and `build.sh` use `cmd/build` for development commands. Website
tooling lives in [`website/`](../website/README.md).
