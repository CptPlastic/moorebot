# Verify SSH into a Scout.
# Confirmed working on Scout (linaro-alip / ROS Melodic): linaro / linaro
# Official root enable (Android app): firmware version tap x5 -> ROOT -> 123456
# Some units also accept root / plt after enabling ROOT in the app.

param(
    [Parameter(Mandatory = $true)]
    [string]$HostAddress,

    [string]$User = "linaro",
    [string]$Password = "linaro",
    [int]$Port = 22
)

$ErrorActionPreference = "Stop"

Write-Host "Testing TCP $HostAddress`:$Port ..."
$tcp = Test-NetConnection -ComputerName $HostAddress -Port $Port -WarningAction SilentlyContinue
if (-not $tcp.TcpTestSucceeded) {
    Write-Host "FAIL: port $Port closed on $HostAddress"
    Write-Host "Enable ROOT in the Android Scout app first, then retry."
    exit 1
}
Write-Host "OK: port $Port open"

$ssh = Get-Command ssh -ErrorAction SilentlyContinue
if (-not $ssh) {
    Write-Host "OpenSSH client not found. Install 'OpenSSH Client' Windows optional feature, then run:"
    Write-Host "  ssh $User@$HostAddress"
    exit 2
}

Write-Host ""
Write-Host "Manual login (password is usually '$Password'):"
Write-Host "  ssh $User@$HostAddress"
Write-Host ""
Write-Host "Once in, run on the robot:"
Write-Host "  bash <(cat)  # or copy probe-ros.sh over"
Write-Host "  # from this repo: soft-hack/probe-ros.sh"
Write-Host ""
Write-Host "Quick one-liner after login:"
Write-Host "  source /opt/ros/*/setup.bash 2>/dev/null; rostopic list"
Write-Host ""

# Attempt non-interactive probe if sshpass exists (rare on Windows).
$sshpass = Get-Command sshpass -ErrorAction SilentlyContinue
if ($sshpass) {
    Write-Host "sshpass found - probing hostname + uname..."
    & sshpass -p $Password ssh -o StrictHostKeyChecking=accept-new "$User@$HostAddress" "hostname; uname -a; which rostopic || true"
} else {
    Write-Host "Connect interactively now if ready:"
    Write-Host "  ssh -o StrictHostKeyChecking=accept-new $User@$HostAddress"
}
