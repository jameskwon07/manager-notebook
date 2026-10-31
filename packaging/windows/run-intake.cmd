@echo off
rem Request inbox only (요청 접수함). Same records as the full notebook.
rem --share also opens the request door (요청 창구) on port 8180 so colleagues can write requests themselves.
rem The notebook screen itself stays on this PC only (127.0.0.1).
"%~dp0manager-notebook.exe" --only intake --share
