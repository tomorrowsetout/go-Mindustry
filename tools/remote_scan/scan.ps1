$root = Resolve-Path (Join-Path $PSScriptRoot "..\..\..\core\src")
$outDir = Resolve-Path (Join-Path $PSScriptRoot "..\..\docs")
$outPath = Join-Path $outDir "remote-methods.csv"

$entries = New-Object System.Collections.Generic.List[object]

Get-ChildItem -Path $root -Recurse -Filter *.java | ForEach-Object {
    $file = $_.FullName
    $rel = $file.Substring($root.Path.Length).TrimStart('\','/')
    $rel = $rel -replace '\\','/'
    $lines = Get-Content $file
    $pending = $null
    $sigBuf = $null
    foreach ($line in $lines) {
        $trim = $line.Trim()
        if ($trim.StartsWith("@Remote")) {
            $pending = $trim
            $sigBuf = $null
            continue
        }
        if ($pending) {
            if ($trim -eq "" -or $trim.StartsWith("@")) {
                continue
            }
            if ($sigBuf -eq $null) {
                $sigBuf = $trim
            } else {
                $sigBuf = ($sigBuf + " " + $trim)
            }
            # support multi-line signatures, e.g. "void foo(" ... ")".
            if ($sigBuf.Contains(")")) {
                $entries.Add([pscustomobject]@{
                    file = $rel
                    remote = $pending
                    signature = $sigBuf
                })
                $pending = $null
                $sigBuf = $null
            }
        }
    }
}

New-Item -ItemType Directory -Force $outDir | Out-Null

$lines = New-Object System.Collections.Generic.List[string]
$lines.Add("file,remote,signature")
$entries | ForEach-Object {
    $line = '"' + $_.file + '","' + $_.remote.Replace('"','""') + '","' + $_.signature.Replace('"','""') + '"'
    $lines.Add($line)
}
[System.IO.File]::WriteAllLines($outPath, $lines, [System.Text.Encoding]::UTF8)

Write-Host ("wrote {0} entries to {1}" -f $entries.Count, $outPath)
