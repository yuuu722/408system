@echo off
title CS408 QA System
cd /d "%~dp0KG_Demo\backend"

set "PYEXE=python"

REM Detect Anaconda Python that already has dependencies
if exist "D:\Anaconda\python.exe" "D:\Anaconda\python.exe" -c "import fastapi" 2>nul && set "PYEXE=D:\Anaconda\python.exe"
if exist "%USERPROFILE%\anaconda3\python.exe" "%USERPROFILE%\anaconda3\python.exe" -c "import fastapi" 2>nul && set "PYEXE=%USERPROFILE%\anaconda3\python.exe"
if exist "%USERPROFILE%\Anaconda3\python.exe" "%USERPROFILE%\Anaconda3\python.exe" -c "import fastapi" 2>nul && set "PYEXE=%USERPROFILE%\Anaconda3\python.exe"
if exist "C:\ProgramData\Anaconda3\python.exe" "C:\ProgramData\Anaconda3\python.exe" -c "import fastapi" 2>nul && set "PYEXE=C:\ProgramData\Anaconda3\python.exe"

echo Using Python: %PYEXE%
echo.

"%PYEXE%" -c "import fastapi" 2>nul
if errorlevel 1 (
    echo [Setup] Installing dependencies, please wait...
    "%PYEXE%" -m pip install -r "%~dp0requirements.txt"
    echo.
)

echo ============================================
echo   CS408 Knowledge Graph QA System
echo.
echo   Open this URL in your browser:
echo          http://localhost:8000
echo.
echo   Close this window to stop the server.
echo ============================================
echo.

"%PYEXE%" main.py

echo.
echo Server stopped.
pause