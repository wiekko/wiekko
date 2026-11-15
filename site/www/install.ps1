# Installs the Nexmap CLI (nexmap.exe) on Windows:
#
#   irm https://nexmap.net/install.ps1 | iex
#
# Environment:
#   NEXMAP_VERSION       version to install, e.g. v1.0.1 (default: the latest release)
#   NEXMAP_INSTALL_DIR   target directory (default: %LOCALAPPDATA%\Programs\nexmap)
#   NEXMAP_RELEASES_URL  where releases are published (default: https://nexmap.net/releases)
#
# Downloads the archive for this platform, checks it against the release's
# SHA256SUMS, installs nexmap.exe and adds the directory to the user's PATH.
# Works with Windows PowerShell 5.1 and PowerShell 7.

& {
    $ErrorActionPreference = 'Stop'
    $ProgressPreference = 'SilentlyContinue'  # makes Invoke-WebRequest much faster on 5.1
    [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12

    $base = if ($env:NEXMAP_RELEASES_URL) { $env:NEXMAP_RELEASES_URL.TrimEnd('/') } else { 'https://nexmap.net/releases' }

    $arch = switch ($env:PROCESSOR_ARCHITECTURE) {
        'AMD64' { 'amd64' }
        'ARM64' { 'arm64' }
        default { throw "nexmap install: unsupported architecture $($env:PROCESSOR_ARCHITECTURE); see $base/" }
    }

    $version = $env:NEXMAP_VERSION
    if (-not $version) {
        $version = (Invoke-RestMethod -UseBasicParsing "$base/latest.json").version
    }
    if ($version -notmatch '^v\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$') {
        throw "nexmap install: '$version' is not a version like v1.2.3"
    }

    $dir = if ($env:NEXMAP_INSTALL_DIR) { $env:NEXMAP_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA 'Programs\nexmap' }
    $name = "nexmap_${version}_windows_${arch}.zip"
    $tmp = Join-Path ([IO.Path]::GetTempPath()) ("nexmap-install-" + [Guid]::NewGuid())
    New-Item -ItemType Directory -Path $tmp | Out-Null
    try {
        Write-Host "Downloading nexmap $version for windows/$arch"
        $sums = (Invoke-WebRequest -UseBasicParsing "$base/$version/SHA256SUMS").Content
        if ($sums -is [byte[]]) { $sums = [Text.Encoding]::UTF8.GetString($sums) }
        $want = $null
        foreach ($line in ($sums -split "`n")) {
            $f = $line.Trim() -split '\s+'
            if ($f.Count -eq 2 -and $f[1].TrimStart('*') -eq $name) { $want = $f[0] }
        }
        if (-not $want) { throw "nexmap install: $version has no build for windows/$arch" }

        $archive = Join-Path $tmp $name
        Invoke-WebRequest -UseBasicParsing "$base/$version/$name" -OutFile $archive
        $got = (Get-FileHash -Algorithm SHA256 $archive).Hash
        if ($got -ne $want.ToUpperInvariant()) {
            throw "nexmap install: checksum mismatch for $name; the download is damaged or was tampered with"
        }

        Expand-Archive -Path $archive -DestinationPath (Join-Path $tmp 'x')
        New-Item -ItemType Directory -Force -Path $dir | Out-Null
        $exe = Join-Path $dir 'nexmap.exe'
        # A running nexmap.exe cannot be overwritten, but it can be renamed.
        if (Test-Path $exe) {
            Remove-Item "$exe.old" -Force -ErrorAction SilentlyContinue
            Move-Item $exe "$exe.old" -Force
        }
        Copy-Item (Join-Path $tmp 'x\nexmap.exe') $exe
        Remove-Item "$exe.old" -Force -ErrorAction SilentlyContinue
        Write-Host "Installed nexmap $version to $exe"

        $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
        $entries = @($userPath -split ';' | Where-Object { $_ })
        if ($entries -notcontains $dir) {
            [Environment]::SetEnvironmentVariable('Path', (($entries + $dir) -join ';'), 'User')
            $env:Path = "$env:Path;$dir"
            Write-Host "Added $dir to your user PATH; open a new terminal to pick it up there."
        }
    }
    finally {
        Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
    }
}
