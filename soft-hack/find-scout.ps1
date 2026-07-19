# Find Moorebot Scout on the local network.
# Scout default WiFi-direct SSID: robot_scout_xxxxxx  password: r0123456
# Prefer AP/router mode so PC and Scout share the same LAN.
# Compatible with Windows PowerShell 5.1+

param(
    [string]$SubnetPrefix = "",
    [int]$TimeoutMs = 400,
    [int]$ThrottleLimit = 64
)

$ErrorActionPreference = "Stop"

function Get-LocalSubnetPrefix {
    $cfg = Get-NetIPConfiguration |
        Where-Object { $_.IPv4DefaultGateway -ne $null -and $_.NetAdapter.Status -eq "Up" } |
        Select-Object -First 1

    if (-not $cfg) {
        throw "No active IPv4 gateway found. Connect this PC to the same WiFi/LAN as Scout."
    }

    $ip = $cfg.IPv4Address.IPAddress
    $parts = $ip.Split(".")
    return "$($parts[0]).$($parts[1]).$($parts[2])"
}

if (-not $SubnetPrefix) {
    $SubnetPrefix = Get-LocalSubnetPrefix
}

Write-Host "Scanning $SubnetPrefix.1-254 for open SSH (22) and ROS master (11311)..."
Write-Host ""

$scriptBlock = {
    param($Ip, $TimeoutMs)

    $ssh = $false
    $ros = $false

    foreach ($port in @(22, 11311)) {
        $client = $null
        try {
            $client = New-Object System.Net.Sockets.TcpClient
            $iar = $client.BeginConnect($Ip, $port, $null, $null)
            $ok = $iar.AsyncWaitHandle.WaitOne($TimeoutMs, $false)
            if ($ok -and $client.Connected) {
                if ($port -eq 22) { $ssh = $true }
                if ($port -eq 11311) { $ros = $true }
            }
        } catch {
            # ignore
        } finally {
            if ($client) { $client.Close() }
        }
    }

    if ($ssh -or $ros) {
        [pscustomobject]@{
            IP          = $Ip
            SSH_22      = $ssh
            ROS_11311   = $ros
            LikelyScout = ($ssh -and $ros)
        }
    }
}

$pool = [runspacefactory]::CreateRunspacePool(1, $ThrottleLimit)
$pool.Open()
$jobs = @()

1..254 | ForEach-Object {
    $ip = "$SubnetPrefix.$_"
    $ps = [powershell]::Create().AddScript($scriptBlock).AddArgument($ip).AddArgument($TimeoutMs)
    $ps.RunspacePool = $pool
    $jobs += [pscustomobject]@{
        Pipe   = $ps
        Handle = $ps.BeginInvoke()
    }
}

$results = @()
foreach ($job in $jobs) {
    $out = $job.Pipe.EndInvoke($job.Handle)
    if ($out) { $results += $out }
    $job.Pipe.Dispose()
}
$pool.Close()
$pool.Dispose()

$results = $results | Sort-Object { [version]$_.IP }

if (-not $results) {
    Write-Host "No hosts with port 22 or 11311 responded."
    Write-Host "Tips:"
    Write-Host "  - Put Scout on home WiFi (router/AP mode) via the Scout app"
    Write-Host "  - Confirm PC is on that same network (not guest/VPN)"
    Write-Host "  - Try: .\find-scout.ps1 -SubnetPrefix 192.168.1"
    exit 1
}

$results | Format-Table -AutoSize

$best = $results | Where-Object LikelyScout | Select-Object -First 1
if ($best) {
    Write-Host "Best candidate: $($best.IP)"
    Write-Host "Next: .\ssh-check.ps1 -HostAddress $($best.IP)"
} else {
    Write-Host "Found open ports, but none had BOTH 22 and 11311."
    Write-Host "If root is not enabled yet, SSH may be closed - finish app root first."
}
