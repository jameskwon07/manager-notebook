@echo off
rem Request inbox only, demo mode: example data in memory, gone when this window closes.
rem --share opens the demo request door on port 8190.
"%~dp0manager-notebook.exe" --only intake --demo --share
