@echo off
rem Daily venue policy refresh for the prototype (Windows Task Scheduler, 00:15).
cd /d "%~dp0.."
"C:\ProgramData\miniconda3\python.exe" -m proto policies refresh >> "..\data\policy-refresh.log" 2>&1
