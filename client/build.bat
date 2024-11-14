@echo off
SETLOCAL

REM Change to the script directory
cd /d "%~dp0"

REM Ensure Python and PyInstaller are installed
echo Checking Python installation...
python --version || (
    echo Python is not installed. Please install Python and add it to your PATH.
    EXIT /B 1
)

echo Checking PyInstaller installation...
pip show pyinstaller >nul 2>&1 || (
    echo Installing PyInstaller...
    pip install pyinstaller
)

REM Run PyInstaller to create the executable
echo Building the executable with PyInstaller...
pyinstaller --onefile --noconsole main.pyw --name earthbound-rom-editor

REM Clean up build files
echo Cleaning up...
del /q build
del /q main.spec

echo Build completed successfully.
pause
ENDLOCAL
