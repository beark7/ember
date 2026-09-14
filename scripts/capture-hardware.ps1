<#
.SYNOPSIS
  Dump raw hardware information for Ember's probe fixtures (Windows).

.DESCRIPTION
  Writes one file per command into testdata\hardware\raw\<Machine>\ plus meta.json.
  Commands that are not available produce a <name>.missing file. No administrator
  rights required; nothing on the system is modified. Host name, user name and
  the home directory are replaced by placeholders before anything is written.
  See docs-private/design/probe.md.

.PARAMETER Machine
  Machine slug, e.g. "pc-4070-32gb". Letters, digits, dot, dash, underscore.

.PARAMETER Out
  Output directory (default: <repo>\testdata\hardware\raw\<Machine>).

.PARAMETER Notes
  Free-text notes stored in meta.json (RAM, GPU, anything odd).

.EXAMPLE
  powershell -ExecutionPolicy Bypass -File scripts\capture-hardware.ps1 -Machine pc-4070-32gb
#>
[CmdletBinding()]
param(
  [Parameter(Mandatory = $true)][string]$Machine,
  [string]$Out = "",
  [string]$Notes = ""
)

$ErrorActionPreference = "Continue"
$ScriptVersion = "1"

if ($Machine -notmatch '^[A-Za-z0-9._-]+$') {
  Write-Error "machine slug must match [A-Za-z0-9._-]+"
  exit 2
}

$RepoRoot = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
if ([string]::IsNullOrEmpty($Out)) {
  $Out = Join-Path $RepoRoot "testdata\hardware\raw\$Machine"
}
New-Item -ItemType Directory -Force -Path $Out | Out-Null

$HostName = $env:COMPUTERNAME
$UserName = $env:USERNAME
$HomeDir = $env:USERPROFILE

function Scrub([string]$text) {
  if ($null -eq $text) { return "" }
  $t = $text
  if ($HomeDir) { $t = $t.Replace($HomeDir, "<HOME>") }
  if ($HostName) { $t = $t -replace [regex]::Escape($HostName), "<HOSTNAME>" }
  if ($UserName) { $t = $t -replace "\b$([regex]::Escape($UserName))\b", "<USER>" }
  return $t
}

function Save([string]$name, [scriptblock]$block) {
  try {
    $output = & $block 2>&1 | Out-String -Width 4096
    [System.IO.File]::WriteAllText((Join-Path $Out $name), (Scrub $output), [System.Text.Encoding]::UTF8)
    Write-Host ("  {0,-36} ok" -f $name)
  } catch {
    [System.IO.File]::WriteAllText((Join-Path $Out "$name.missing"), (Scrub $_.ToString()), [System.Text.Encoding]::UTF8)
    Write-Host ("  {0,-36} missing ({1})" -f $name, $_.Exception.Message)
  }
}

function SaveCommand([string]$name, [string]$exe, [string[]]$arguments) {
  $cmd = Get-Command $exe -ErrorAction SilentlyContinue
  if (-not $cmd) {
    [System.IO.File]::WriteAllText((Join-Path $Out "$name.missing"), "$exe: command not found", [System.Text.Encoding]::UTF8)
    Write-Host ("  {0,-36} missing ({1})" -f $name, $exe)
    return
  }
  Save $name { & $exe @arguments }
}

Write-Host "Capturing hardware info for '$Machine' (windows) into $Out"

Save "date.txt" { (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ") }
Save "os.json" { Get-CimInstance Win32_OperatingSystem | Select-Object Caption, Version, BuildNumber, OSArchitecture, TotalVisibleMemorySize, FreePhysicalMemory | ConvertTo-Json }
Save "computer_system.json" { Get-CimInstance Win32_ComputerSystem | Select-Object Manufacturer, Model, SystemType, TotalPhysicalMemory, NumberOfProcessors, NumberOfLogicalProcessors, PCSystemType | ConvertTo-Json }
Save "processor.json" { Get-CimInstance Win32_Processor | Select-Object Name, Manufacturer, NumberOfCores, NumberOfLogicalProcessors, MaxClockSpeed, L2CacheSize, L3CacheSize, Architecture | ConvertTo-Json }
Save "physical_memory.json" { Get-CimInstance Win32_PhysicalMemory | Select-Object Capacity, Speed, ConfiguredClockSpeed, MemoryType, SMBIOSMemoryType, DeviceLocator | ConvertTo-Json }
Save "video_controller.json" { Get-CimInstance Win32_VideoController | Select-Object Name, AdapterCompatibility, AdapterRAM, DriverVersion, VideoProcessor, PNPDeviceID | ConvertTo-Json }
Save "physical_disk.json" { Get-PhysicalDisk | Select-Object FriendlyName, MediaType, BusType, Size, HealthStatus | ConvertTo-Json }
Save "logical_disk.json" { Get-CimInstance Win32_LogicalDisk | Select-Object DeviceID, DriveType, Size, FreeSpace, FileSystem | ConvertTo-Json }
Save "battery.json" { Get-CimInstance Win32_Battery | Select-Object Name, BatteryStatus, EstimatedChargeRemaining, DesignCapacity | ConvertTo-Json }
Save "net_adapter.json" { Get-NetAdapter -Physical | Select-Object Name, InterfaceDescription, Status, LinkSpeed, MacAddress | ConvertTo-Json }
Save "env_processor.txt" { "PROCESSOR_IDENTIFIER=$env:PROCESSOR_IDENTIFIER"; "PROCESSOR_ARCHITECTURE=$env:PROCESSOR_ARCHITECTURE"; "NUMBER_OF_PROCESSORS=$env:NUMBER_OF_PROCESSORS" }

SaveCommand "nvidia_smi.csv" "nvidia-smi" @("--query-gpu=index,name,memory.total,memory.used,memory.free,driver_version,compute_cap,pci.bus_id", "--format=csv")
SaveCommand "nvidia_smi.txt" "nvidia-smi" @()
SaveCommand "vulkaninfo_summary.txt" "vulkaninfo" @("--summary")
SaveCommand "dxdiag.txt" "dxdiag" @("/t", (Join-Path $Out "dxdiag.tmp"))
if (Test-Path (Join-Path $Out "dxdiag.tmp")) {
  $dx = Get-Content (Join-Path $Out "dxdiag.tmp") -Raw
  [System.IO.File]::WriteAllText((Join-Path $Out "dxdiag.txt"), (Scrub $dx), [System.Text.Encoding]::UTF8)
  Remove-Item (Join-Path $Out "dxdiag.tmp") -Force
}

# llama-server: PATH or the Ember engines directory.
$llama = Get-Command "llama-server.exe" -ErrorAction SilentlyContinue
if (-not $llama) {
  $enginesDir = if ($env:EMBER_HOME) { Join-Path $env:EMBER_HOME "engines" } else { Join-Path $env:APPDATA "Ember\engines" }
  if (Test-Path $enginesDir) {
    $llama = Get-ChildItem -Path $enginesDir -Recurse -Filter "llama-server.exe" -ErrorAction SilentlyContinue | Select-Object -First 1
  }
}
if ($llama) {
  $exe = if ($llama.PSObject.Properties["FullName"]) { $llama.FullName } else { $llama.Source }
  Save "llama_list_devices.txt" { & $exe --list-devices }
  Save "llama_server_version.txt" { & $exe --version }
} else {
  [System.IO.File]::WriteAllText((Join-Path $Out "llama_list_devices.txt.missing"), "llama-server: not found in PATH or engines dir", [System.Text.Encoding]::UTF8)
  Write-Host ("  {0,-36} missing (no engine installed yet)" -f "llama_list_devices.txt")
}

$meta = [ordered]@{
  schema         = 1
  script         = "capture-hardware.ps1"
  script_version = $ScriptVersion
  machine        = $Machine
  os             = "windows"
  arch           = $env:PROCESSOR_ARCHITECTURE
  captured_at    = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
  notes          = $Notes
}
[System.IO.File]::WriteAllText((Join-Path $Out "meta.json"), ($meta | ConvertTo-Json), [System.Text.Encoding]::UTF8)
Write-Host ("  {0,-36} ok" -f "meta.json")

Write-Host ""
Write-Host "Done. Review the files in $Out for anything personal before committing."
Write-Host "Please add RAM, GPU and any oddities to meta.json 'notes'."
