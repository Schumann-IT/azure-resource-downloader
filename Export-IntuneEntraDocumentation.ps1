<#
.SYNOPSIS
    Exports the complete Intune / Autopilot / Entra ID configuration of a tenant
    as machine-readable JSON (for documentation/diffing/review).

.DESCRIPTION
    Designed for a "modern workplace" baseline documentation (Windows + macOS clients).

    Exported categories:
      Intune - Device configuration:
        * Device configuration profiles (classic templates incl. custom OMA-URI / custom .mobileconfig)
        * Settings Catalog policies (incl. new Endpoint Security policies, with full settings body)
        * Administrative Templates (Group Policy configurations, incl. definition values)
        * Endpoint Security intents (legacy security baselines / AV / BitLocker etc., with settings)
        * Compliance policies (classic + settings-catalog based)
        * Windows Update rings (part of classic profiles) + Feature / Quality / Driver update profiles
        * Assignment filters
      Intune - Scripts:
        * Windows platform scripts (PowerShell, decoded to .ps1)
        * macOS shell scripts (decoded to .sh)
        * macOS custom attribute scripts
        * Remediations (proactive remediations, detection + remediation scripts decoded)
      Autopilot / Enrollment:
        * Autopilot deployment profiles (incl. OOBE/onboarding screen settings) + assignments
        * Autopilot device identities incl. group tags
        * Enrollment configurations (Enrollment Status Page, enrollment restrictions,
          Windows Hello for Business, enrollment notifications, device limits)
        * Apple enrollment (ADE/DEP tokens + profiles, Apple MDM push certificate, user-initiated enrollment)
      Intune - Apps:
        * All applications incl. assignments
        * App protection policies (iOS/Android/Windows, WIP)
        * App configuration policies (managed devices + managed apps)
      Tenant / Intune admin:
        * Intune tenant settings (deviceManagement object)
        * Device categories, scope tags, Intune role definitions + assignments
        * Terms & Conditions, Intune branding profiles, notification message templates
      Entra ID:
        * Conditional Access policies + named locations + authentication strengths + terms of use
        * Authentication methods policy (MFA/SSPR registration methods, registration campaign)
        * Authorization policy (incl. SSPR-for-admins flag) + SSPR notes
        * Entra Connect sync configuration (onPremisesSynchronization features) + organization info
        * All groups referenced anywhere in the above (resolved with type, dynamic membership rules)
        * All dynamic groups in the tenant (full list with rules)

    Output structure:
      <OutputPath>\Json\<Category>\_all.json        -> full raw Graph data per category
      <OutputPath>\Json\<Category>\<item>.json      -> per-item raw data (where useful)
      <OutputPath>\Json\_exportManifest.json        -> index: tenant info, per-category item
                                                       summaries with resolved group names,
                                                       export errors, manual-documentation to-dos
      <OutputPath>\Scripts\...                      -> decoded script payloads (.ps1/.sh)

.REQUIREMENTS
    - Windows Server 2019, Windows PowerShell 5.1 (PowerShell 7 also works)
    - Microsoft Graph PowerShell SDK installed (only Microsoft.Graph.Authentication is required):
        Install-Module Microsoft.Graph.Authentication -Scope AllUsers
    - Run as a user with Global Administrator (read access via granted Graph scopes)

.EXAMPLE
    .\Export-IntuneEntraDocumentation.ps1 -OutputPath C:\IntuneDocs
#>

[CmdletBinding()]
param(
    [string]$OutputPath = (Join-Path -Path "C:\IntuneDocs" -ChildPath ("Export_{0}" -f (Get-Date -Format 'yyyy-MM-dd_HHmm'))),

    # Skip exporting the (potentially large) Autopilot device identity list
    [switch]$SkipAutopilotDevices,

    # Disconnect the Graph session at the end
    [switch]$DisconnectWhenDone
)

#region ── Setup ─────────────────────────────────────────────────────────────────

Set-StrictMode -Off
$ErrorActionPreference = 'Stop'
[Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12

$GraphBase   = 'https://graph.microsoft.com/beta'
$GraphV1     = 'https://graph.microsoft.com/v1.0'

$script:Errors     = New-Object System.Collections.Generic.List[string]
$script:GroupIds   = New-Object 'System.Collections.Generic.HashSet[string]'
$script:GroupUsage = @{}   # groupId -> List[string] of "Category: ItemName"
$script:GroupLookup = @{}  # groupId -> resolved group object
$script:Summaries  = [ordered]@{}  # title -> @{ File; Items; Notes }
$script:Counts     = [ordered]@{}

if (-not (Get-Module -ListAvailable -Name Microsoft.Graph.Authentication)) {
    throw "Microsoft Graph PowerShell SDK not found. Install it with: Install-Module Microsoft.Graph.Authentication -Scope AllUsers"
}
Import-Module Microsoft.Graph.Authentication -ErrorAction Stop

$JsonRoot     = Join-Path $OutputPath 'Json'
$ScriptsRoot  = Join-Path $OutputPath 'Scripts'
foreach ($p in @($OutputPath, $JsonRoot, $ScriptsRoot)) {
    if (-not (Test-Path $p)) { New-Item -ItemType Directory -Path $p -Force | Out-Null }
}

#endregion

#region ── Helper functions ─────────────────────────────────────────────────────

function Invoke-GraphGetAll {
    <# Paged GET, returns array of items (follows @odata.nextLink) #>
    param([Parameter(Mandatory)][string]$Uri, [hashtable]$Headers)
    $results = @()
    $next = $Uri
    while ($next) {
        if ($Headers) { $resp = Invoke-MgGraphRequest -Method GET -Uri $next -Headers $Headers -OutputType PSObject }
        else          { $resp = Invoke-MgGraphRequest -Method GET -Uri $next -OutputType PSObject }
        if ($resp.PSObject.Properties.Name -contains 'value') { $results += @($resp.value) }
        else { $results += $resp }
        $next = $null
        if ($resp.PSObject.Properties.Name -contains '@odata.nextLink') { $next = $resp.'@odata.nextLink' }
    }
    return ,$results   # comma operator: keep empty arrays from unrolling to $null
}

function Invoke-GraphGet {
    <# Single-object GET, returns $null on failure #>
    param([Parameter(Mandatory)][string]$Uri)
    try { return Invoke-MgGraphRequest -Method GET -Uri $Uri -OutputType PSObject }
    catch { return $null }
}

function Get-SafeFileName {
    param([string]$Name, [int]$MaxLength = 80)
    if ([string]::IsNullOrWhiteSpace($Name)) { $Name = 'unnamed' }
    # explicit invalid set (deterministic on any OS, matches Windows rules + control chars)
    $clean = ($Name -replace '[\\/:*?"<>|\x00-\x1F]', '_').Trim()
    if ($clean.Length -gt $MaxLength) { $clean = $clean.Substring(0, $MaxLength) }
    return $clean
}

function Save-Json {
    param($Object, [Parameter(Mandatory)][string]$Category, [string]$FileName = '_all.json')
    if ($null -eq $Object) { $Object = @() }
    $dir = Join-Path $JsonRoot $Category
    if (-not (Test-Path $dir)) { New-Item -ItemType Directory -Path $dir -Force | Out-Null }
    $path = Join-Path $dir $FileName
    # -InputObject keeps empty arrays as [] instead of writing an empty file
    ConvertTo-Json -InputObject $Object -Depth 50 | Out-File -FilePath $path -Encoding UTF8
}

function Save-ItemJson {
    param($Item, [string]$Category)
    $name = $Item.displayName
    if (-not $name) { $name = $Item.name }
    $file = "{0}_{1}.json" -f (Get-SafeFileName $name), ([string]$Item.id).Substring(0, 8)
    Save-Json -Object $Item -Category $Category -FileName $file
}

function Add-GroupRef {
    param([string]$GroupId, [string]$Context)
    if ([string]::IsNullOrWhiteSpace($GroupId)) { return }
    [void]$script:GroupIds.Add($GroupId)
    if (-not $script:GroupUsage.ContainsKey($GroupId)) {
        $script:GroupUsage[$GroupId] = New-Object System.Collections.Generic.List[string]
    }
    if ($Context -and -not $script:GroupUsage[$GroupId].Contains($Context)) {
        $script:GroupUsage[$GroupId].Add($Context)
    }
}

function ConvertTo-AssignmentSummary {
    <# Converts Graph assignment objects into a readable string.
       Group names are inserted later via {{GROUP:<id>}} tokens. #>
    param($Assignments, [string]$Context)
    $out = New-Object System.Collections.Generic.List[string]
    foreach ($a in @($Assignments)) {
        if ($null -eq $a) { continue }
        $t = $null
        if ($a.PSObject.Properties.Name -contains 'target') { $t = $a.target }
        if ($null -eq $t) { continue }
        $odataType = ''
        if ($t.PSObject.Properties.Name -contains '@odata.type') { $odataType = [string]$t.'@odata.type' }
        $intent = ''
        if (($a.PSObject.Properties.Name -contains 'intent') -and $a.intent) { $intent = "[$($a.intent)] " }
        $filter = ''
        if (($t.PSObject.Properties.Name -contains 'deviceAndAppManagementAssignmentFilterId') -and $t.deviceAndAppManagementAssignmentFilterId) {
            $filter = " (filter: $($t.deviceAndAppManagementAssignmentFilterType))"
        }
        if     ($odataType -like '*allDevicesAssignmentTarget')       { $out.Add("${intent}All Devices$filter") }
        elseif ($odataType -like '*allLicensedUsersAssignmentTarget') { $out.Add("${intent}All Users$filter") }
        elseif ($odataType -like '*exclusionGroupAssignmentTarget')   { Add-GroupRef $t.groupId $Context; $out.Add("${intent}EXCLUDE {{GROUP:$($t.groupId)}}$filter") }
        elseif ($odataType -like '*groupAssignmentTarget')            { Add-GroupRef $t.groupId $Context; $out.Add("${intent}{{GROUP:$($t.groupId)}}$filter") }
        else { $out.Add("${intent}$odataType") }
    }
    if ($out.Count -eq 0) { return 'Not assigned' }
    return ($out -join '; ')
}

function Get-ItemAssignments {
    param([string]$BaseUri, [string]$Id)
    try { return Invoke-GraphGetAll "$BaseUri/$Id/assignments" } catch { return @() }
}

function Get-PlatformFromODataType {
    param([string]$Type)
    $t = ([string]$Type).ToLower()
    if     ($t -match 'windows' -or $t -match 'sharedpc' -or $t -match 'editionupgrade') { return 'Windows' }
    elseif ($t -match 'macos')   { return 'macOS' }
    elseif ($t -match 'ios')     { return 'iOS/iPadOS' }
    elseif ($t -match 'android') { return 'Android' }
    elseif ($t -match 'aosp')    { return 'Android (AOSP)' }
    return ''
}

function New-SummaryItem {
    param([string]$Name, [string]$Type, [string]$Platform, [string]$Modified, [string]$Assignments, [string]$Extra)
    [PSCustomObject]@{
        Name = $Name; Type = $Type; Platform = $Platform
        Modified = $Modified; Assignments = $Assignments; Extra = $Extra
    }
}

function Add-Summary {
    param([string]$Title, $Items, [string]$Notes, [string[]]$ExtraColumns)
    $script:Summaries[$Title] = @{ Items = @($Items); Notes = $Notes; ExtraColumns = $ExtraColumns }
    $script:Counts[$Title] = @($Items).Count
}

function ConvertFrom-Base64Script {
    param([string]$Base64)
    if ([string]::IsNullOrWhiteSpace($Base64)) { return '' }
    try { return [Text.Encoding]::UTF8.GetString([Convert]::FromBase64String($Base64)) } catch { return '' }
}

function Save-ScriptContent {
    param([string]$SubFolder, [string]$FileName, [string]$Content)
    $dir = Join-Path $ScriptsRoot $SubFolder
    if (-not (Test-Path $dir)) { New-Item -ItemType Directory -Path $dir -Force | Out-Null }
    $Content | Out-File -FilePath (Join-Path $dir $FileName) -Encoding UTF8
}

function Invoke-Section {
    param([string]$Name, [scriptblock]$Body)
    Write-Host "==> $Name" -ForegroundColor Cyan
    try { & $Body }
    catch {
        $msg = $_.Exception.Message
        Write-Warning "Section '$Name' failed: $msg"
        $script:Errors.Add("$Name : $msg")
    }
}

function Get-GroupLabel {
    param([string]$GroupId)
    if ($script:GroupLookup.ContainsKey($GroupId)) {
        $g = $script:GroupLookup[$GroupId]
        $tag = ''
        if ($g.PSObject.Properties.Name -contains 'membershipRule' -and $g.membershipRule) { $tag = ' (dynamic)' }
        return "$($g.displayName)$tag"
    }
    return "Unknown/deleted group $GroupId"
}

function Expand-GroupTokens {
    param([string]$Text)
    return [regex]::Replace($Text, '\{\{GROUP:([0-9a-fA-F\-]+)\}\}', {
        param($m) Get-GroupLabel $m.Groups[1].Value
    })
}

#endregion

#region ── Connect ──────────────────────────────────────────────────────────────

$Scopes = @(
    'DeviceManagementConfiguration.Read.All'
    'DeviceManagementApps.Read.All'
    'DeviceManagementServiceConfig.Read.All'
    'DeviceManagementManagedDevices.Read.All'
    'DeviceManagementRBAC.Read.All'
    'Policy.Read.All'
    'Directory.Read.All'
    'Group.Read.All'
    'Organization.Read.All'
    'OnPremDirectorySynchronization.Read.All'
    'Agreement.Read.All'
)

Write-Host "Connecting to Microsoft Graph (modern authentication prompt will appear)..." -ForegroundColor Yellow
Connect-MgGraph -Scopes $Scopes -NoWelcome
$ctx = Get-MgContext
Write-Host ("Connected as {0} (tenant {1})" -f $ctx.Account, $ctx.TenantId) -ForegroundColor Green

#endregion

#region ── 1. Intune: Device configuration policies ─────────────────────────────

Invoke-Section 'Device configuration profiles (classic templates)' {
    $items = Invoke-GraphGetAll "$GraphBase/deviceManagement/deviceConfigurations"
    $summary = @()
    foreach ($i in $items) {
        $i | Add-Member -NotePropertyName 'assignments' -NotePropertyValue (Get-ItemAssignments "$GraphBase/deviceManagement/deviceConfigurations" $i.id) -Force
        Save-ItemJson $i 'DeviceConfigurations'
        $type = ([string]$i.'@odata.type') -replace '#microsoft\.graph\.', ''
        $summary += New-SummaryItem -Name $i.displayName -Type $type -Platform (Get-PlatformFromODataType $type) `
            -Modified $i.lastModifiedDateTime -Assignments (ConvertTo-AssignmentSummary $i.assignments "Device configuration: $($i.displayName)")
    }
    Save-Json $items 'DeviceConfigurations'
    Add-Summary 'Device Configuration Profiles (classic)' $summary `
        'Classic template-based profiles, including custom OMA-URI (Windows) and custom .mobileconfig (macOS) profiles, and Windows Update rings. Full settings: Json\DeviceConfigurations\.'
}

Invoke-Section 'Settings Catalog policies (incl. new Endpoint Security)' {
    $items = Invoke-GraphGetAll "$GraphBase/deviceManagement/configurationPolicies"
    $summary = @()
    foreach ($i in $items) {
        $i | Add-Member -NotePropertyName 'assignments' -NotePropertyValue (Get-ItemAssignments "$GraphBase/deviceManagement/configurationPolicies" $i.id) -Force
        $settings = @()
        try { $settings = Invoke-GraphGetAll "$GraphBase/deviceManagement/configurationPolicies/$($i.id)/settings" } catch {}
        $i | Add-Member -NotePropertyName 'settings' -NotePropertyValue $settings -Force
        Save-ItemJson $i 'SettingsCatalog'
        $tmpl = ''
        if ($i.PSObject.Properties.Name -contains 'templateReference' -and $i.templateReference -and $i.templateReference.templateDisplayName) {
            $tmpl = $i.templateReference.templateDisplayName
        }
        $summary += New-SummaryItem -Name $i.name -Type ("Settings Catalog" + $(if ($tmpl) { " / $tmpl" })) `
            -Platform ([string]$i.platforms) -Modified $i.lastModifiedDateTime `
            -Assignments (ConvertTo-AssignmentSummary $i.assignments "Settings Catalog: $($i.name)") `
            -Extra ("{0} settings" -f $i.settingCount)
    }
    Save-Json $items 'SettingsCatalog'
    Add-Summary 'Settings Catalog Policies (new)' $summary `
        'Settings Catalog policies, including the modern Endpoint Security policies (AV, Firewall, BitLocker/FileVault, ASR, EDR). Full setting instances are in Json\SettingsCatalog\.' @('Extra')
}

Invoke-Section 'Administrative Templates (Group Policy configurations)' {
    $items = Invoke-GraphGetAll "$GraphBase/deviceManagement/groupPolicyConfigurations"
    $summary = @()
    foreach ($i in $items) {
        $i | Add-Member -NotePropertyName 'assignments' -NotePropertyValue (Get-ItemAssignments "$GraphBase/deviceManagement/groupPolicyConfigurations" $i.id) -Force
        $defs = @()
        try { $defs = Invoke-GraphGetAll "$GraphBase/deviceManagement/groupPolicyConfigurations/$($i.id)/definitionValues?`$expand=definition" } catch {}
        $i | Add-Member -NotePropertyName 'definitionValues' -NotePropertyValue $defs -Force
        Save-ItemJson $i 'AdministrativeTemplates'
        $summary += New-SummaryItem -Name $i.displayName -Type 'Administrative Template' -Platform 'Windows' `
            -Modified $i.lastModifiedDateTime -Assignments (ConvertTo-AssignmentSummary $i.assignments "Admin template: $($i.displayName)") `
            -Extra ("{0} configured settings" -f @($defs).Count)
    }
    Save-Json $items 'AdministrativeTemplates'
    Add-Summary 'Administrative Templates' $summary `
        'Group Policy (ADMX) based profiles. Configured definition values incl. enabled/disabled state are in Json\AdministrativeTemplates\.' @('Extra')
}

Invoke-Section 'Endpoint Security intents (legacy baselines/templates)' {
    $templates = @{}
    try { foreach ($t in (Invoke-GraphGetAll "$GraphBase/deviceManagement/templates")) { $templates[$t.id] = $t.displayName } } catch {}
    $items = Invoke-GraphGetAll "$GraphBase/deviceManagement/intents"
    $summary = @()
    foreach ($i in $items) {
        $i | Add-Member -NotePropertyName 'assignments' -NotePropertyValue (Get-ItemAssignments "$GraphBase/deviceManagement/intents" $i.id) -Force
        $settings = @()
        try { $settings = Invoke-GraphGetAll "$GraphBase/deviceManagement/intents/$($i.id)/settings" } catch {}
        $i | Add-Member -NotePropertyName 'settings' -NotePropertyValue $settings -Force
        Save-ItemJson $i 'EndpointSecurityIntents'
        $tmplName = ''
        if ($i.templateId -and $templates.ContainsKey($i.templateId)) { $tmplName = $templates[$i.templateId] }
        $summary += New-SummaryItem -Name $i.displayName -Type ("Intent" + $(if ($tmplName) { " / $tmplName" })) -Platform '' `
            -Modified $i.lastModifiedDateTime -Assignments (ConvertTo-AssignmentSummary $i.assignments "Endpoint Security intent: $($i.displayName)")
    }
    Save-Json $items 'EndpointSecurityIntents'
    Add-Summary 'Endpoint Security Intents (legacy)' $summary `
        'Legacy template-based Endpoint Security policies and Security Baselines. Newer Endpoint Security policies appear under Settings Catalog. Settings in Json\EndpointSecurityIntents\.'
}

Invoke-Section 'Compliance policies' {
    $items = Invoke-GraphGetAll "$GraphBase/deviceManagement/deviceCompliancePolicies?`$expand=scheduledActionsForRule(`$expand=scheduledActionConfigurations)"
    $summary = @()
    foreach ($i in $items) {
        $i | Add-Member -NotePropertyName 'assignments' -NotePropertyValue (Get-ItemAssignments "$GraphBase/deviceManagement/deviceCompliancePolicies" $i.id) -Force
        Save-ItemJson $i 'CompliancePolicies'
        $type = ([string]$i.'@odata.type') -replace '#microsoft\.graph\.', ''
        $summary += New-SummaryItem -Name $i.displayName -Type $type -Platform (Get-PlatformFromODataType $type) `
            -Modified $i.lastModifiedDateTime -Assignments (ConvertTo-AssignmentSummary $i.assignments "Compliance: $($i.displayName)")
    }
    Save-Json $items 'CompliancePolicies'

    # New settings-catalog based compliance policies (e.g. Linux, newer platforms)
    $v2 = @()
    try {
        $v2 = Invoke-GraphGetAll "$GraphBase/deviceManagement/compliancePolicies"
        foreach ($i in $v2) {
            $i | Add-Member -NotePropertyName 'assignments' -NotePropertyValue (Get-ItemAssignments "$GraphBase/deviceManagement/compliancePolicies" $i.id) -Force
            $settings = @()
            try { $settings = Invoke-GraphGetAll "$GraphBase/deviceManagement/compliancePolicies/$($i.id)/settings" } catch {}
            $i | Add-Member -NotePropertyName 'settings' -NotePropertyValue $settings -Force
            $summary += New-SummaryItem -Name $i.name -Type 'Compliance (Settings Catalog)' -Platform ([string]$i.platforms) `
                -Modified $i.lastModifiedDateTime -Assignments (ConvertTo-AssignmentSummary $i.assignments "Compliance v2: $($i.name)")
        }
        Save-Json $v2 'CompliancePolicies' '_all_settingsCatalogBased.json'
    } catch {}
    Add-Summary 'Compliance Policies' $summary `
        'Device compliance policies incl. scheduled actions (grace periods, noncompliance actions). Full rules in Json\CompliancePolicies\.'
}

Invoke-Section 'Windows Update profiles (Feature/Quality/Driver)' {
    $summary = @()
    $map = @(
        @{ Uri = 'windowsFeatureUpdateProfiles'; Type = 'Feature update profile' },
        @{ Uri = 'windowsQualityUpdateProfiles'; Type = 'Quality update profile' },
        @{ Uri = 'windowsDriverUpdateProfiles';  Type = 'Driver update profile' }
    )
    $all = @{}
    foreach ($m in $map) {
        $items = @()
        try { $items = Invoke-GraphGetAll "$GraphBase/deviceManagement/$($m.Uri)" } catch { continue }
        foreach ($i in $items) {
            $i | Add-Member -NotePropertyName 'assignments' -NotePropertyValue (Get-ItemAssignments "$GraphBase/deviceManagement/$($m.Uri)" $i.id) -Force
            $summary += New-SummaryItem -Name $i.displayName -Type $m.Type -Platform 'Windows' `
                -Modified $i.lastModifiedDateTime -Assignments (ConvertTo-AssignmentSummary $i.assignments "$($m.Type): $($i.displayName)")
        }
        $all[$m.Uri] = $items
    }
    Save-Json $all 'WindowsUpdateProfiles'
    Add-Summary 'Windows Update Profiles' $summary `
        'Feature, quality (expedite) and driver update profiles. Update RINGS are classic device configuration profiles (type windowsUpdateForBusinessConfiguration) - see Device Configuration Profiles.'
}

Invoke-Section 'Assignment filters' {
    $items = Invoke-GraphGetAll "$GraphBase/deviceManagement/assignmentFilters"
    Save-Json $items 'AssignmentFilters'
    $summary = foreach ($i in $items) {
        New-SummaryItem -Name $i.displayName -Type ([string]$i.assignmentFilterManagementType) -Platform ([string]$i.platform) `
            -Modified $i.lastModifiedDateTime -Assignments '-' -Extra ([string]$i.rule)
    }
    Add-Summary 'Assignment Filters' $summary 'Device/app assignment filters with their rule syntax.' @('Extra')
}

#endregion

#region ── 2. Intune: Scripts ───────────────────────────────────────────────────

Invoke-Section 'Windows platform scripts (PowerShell)' {
    $items = Invoke-GraphGetAll "$GraphBase/deviceManagement/deviceManagementScripts"
    $summary = @()
    $full = @()
    foreach ($i in $items) {
        $d = Invoke-GraphGet "$GraphBase/deviceManagement/deviceManagementScripts/$($i.id)"
        if ($null -eq $d) { $d = $i }
        $d | Add-Member -NotePropertyName 'assignments' -NotePropertyValue (Get-ItemAssignments "$GraphBase/deviceManagement/deviceManagementScripts" $i.id) -Force
        $content = ConvertFrom-Base64Script $d.scriptContent
        if ($content) { Save-ScriptContent 'PlatformScripts_Windows' ("{0}.ps1" -f (Get-SafeFileName $d.displayName)) $content }
        Save-ItemJson $d 'PlatformScriptsWindows'
        $full += $d
        $summary += New-SummaryItem -Name $d.displayName -Type 'PowerShell script' -Platform 'Windows' `
            -Modified $d.lastModifiedDateTime -Assignments (ConvertTo-AssignmentSummary $d.assignments "Win script: $($d.displayName)") `
            -Extra ("RunAs: {0}; 64bit: {1}; SigCheck: {2}" -f $d.runAsAccount, $d.runAs32Bit, $d.enforceSignatureCheck)
    }
    Save-Json $full 'PlatformScriptsWindows'
    Add-Summary 'Platform Scripts - Windows' $summary `
        'Intune PowerShell platform scripts. Decoded script bodies: Scripts\PlatformScripts_Windows\.' @('Extra')
}

Invoke-Section 'macOS shell scripts' {
    $items = Invoke-GraphGetAll "$GraphBase/deviceManagement/deviceShellScripts"
    $summary = @()
    $full = @()
    foreach ($i in $items) {
        $d = Invoke-GraphGet "$GraphBase/deviceManagement/deviceShellScripts/$($i.id)"
        if ($null -eq $d) { $d = $i }
        $d | Add-Member -NotePropertyName 'assignments' -NotePropertyValue (Get-ItemAssignments "$GraphBase/deviceManagement/deviceShellScripts" $i.id) -Force
        $content = ConvertFrom-Base64Script $d.scriptContent
        if ($content) { Save-ScriptContent 'ShellScripts_macOS' ("{0}.sh" -f (Get-SafeFileName $d.displayName)) $content }
        Save-ItemJson $d 'ShellScriptsMacOS'
        $full += $d
        $summary += New-SummaryItem -Name $d.displayName -Type 'Shell script' -Platform 'macOS' `
            -Modified $d.lastModifiedDateTime -Assignments (ConvertTo-AssignmentSummary $d.assignments "macOS script: $($d.displayName)") `
            -Extra ("RunAs: {0}; Frequency: {1}; Retries: {2}" -f $d.runAsAccount, $d.executionFrequency, $d.retryCount)
    }
    Save-Json $full 'ShellScriptsMacOS'
    Add-Summary 'Shell Scripts - macOS' $summary `
        'Intune macOS shell scripts. Decoded script bodies: Scripts\ShellScripts_macOS\.' @('Extra')
}

Invoke-Section 'macOS custom attribute scripts' {
    $items = Invoke-GraphGetAll "$GraphBase/deviceManagement/deviceCustomAttributeShellScripts"
    $summary = @()
    $full = @()
    foreach ($i in $items) {
        $d = Invoke-GraphGet "$GraphBase/deviceManagement/deviceCustomAttributeShellScripts/$($i.id)"
        if ($null -eq $d) { $d = $i }
        $d | Add-Member -NotePropertyName 'assignments' -NotePropertyValue (Get-ItemAssignments "$GraphBase/deviceManagement/deviceCustomAttributeShellScripts" $i.id) -Force
        $content = ConvertFrom-Base64Script $d.scriptContent
        if ($content) { Save-ScriptContent 'CustomAttributes_macOS' ("{0}.sh" -f (Get-SafeFileName $d.displayName)) $content }
        $full += $d
        $summary += New-SummaryItem -Name $d.displayName -Type ("Custom attribute ({0})" -f $d.customAttributeType) -Platform 'macOS' `
            -Modified $d.lastModifiedDateTime -Assignments (ConvertTo-AssignmentSummary $d.assignments "macOS custom attribute: $($d.displayName)")
    }
    Save-Json $full 'CustomAttributesMacOS'
    Add-Summary 'Custom Attributes - macOS' $summary 'macOS custom attribute scripts. Bodies: Scripts\CustomAttributes_macOS\.'
}

Invoke-Section 'Remediations (proactive remediation scripts)' {
    $items = Invoke-GraphGetAll "$GraphBase/deviceManagement/deviceHealthScripts"
    $summary = @()
    $full = @()
    foreach ($i in $items) {
        $d = Invoke-GraphGet "$GraphBase/deviceManagement/deviceHealthScripts/$($i.id)"
        if ($null -eq $d) { $d = $i }
        $d | Add-Member -NotePropertyName 'assignments' -NotePropertyValue (Get-ItemAssignments "$GraphBase/deviceManagement/deviceHealthScripts" $i.id) -Force
        $det = ConvertFrom-Base64Script $d.detectionScriptContent
        $rem = ConvertFrom-Base64Script $d.remediationScriptContent
        $base = Get-SafeFileName $d.displayName
        if ($det) { Save-ScriptContent 'Remediations' ("{0}_detect.ps1" -f $base) $det }
        if ($rem) { Save-ScriptContent 'Remediations' ("{0}_remediate.ps1" -f $base) $rem }
        Save-ItemJson $d 'Remediations'
        $full += $d
        $summary += New-SummaryItem -Name $d.displayName -Type ("Remediation (publisher: {0})" -f $d.publisher) -Platform 'Windows' `
            -Modified $d.lastModifiedDateTime -Assignments (ConvertTo-AssignmentSummary $d.assignments "Remediation: $($d.displayName)") `
            -Extra ("RunAs: {0}; 32bit: {1}" -f $d.runAsAccount, $d.runAs32Bit)
    }
    Save-Json $full 'Remediations'
    Add-Summary 'Remediations' $summary `
        'Proactive remediations. Decoded detection/remediation scripts: Scripts\Remediations\.' @('Extra')
}

#endregion

#region ── 3. Autopilot & Enrollment ────────────────────────────────────────────

Invoke-Section 'Autopilot deployment profiles' {
    $items = Invoke-GraphGetAll "$GraphBase/deviceManagement/windowsAutopilotDeploymentProfiles"
    $summary = @()
    foreach ($i in $items) {
        $i | Add-Member -NotePropertyName 'assignments' -NotePropertyValue (Get-ItemAssignments "$GraphBase/deviceManagement/windowsAutopilotDeploymentProfiles" $i.id) -Force
        Save-ItemJson $i 'AutopilotProfiles'
        $oobe = ''
        if ($i.PSObject.Properties.Name -contains 'outOfBoxExperienceSetting' -and $i.outOfBoxExperienceSetting) {
            $o = $i.outOfBoxExperienceSetting
            $oobe = "UserType: $($o.userType); Privacy: $($o.privacySettingsHidden); EULA: $($o.eulaHidden); KeyboardSkip: $($o.keyboardSelectionPageSkipped)"
        } elseif ($i.PSObject.Properties.Name -contains 'outOfBoxExperienceSettings' -and $i.outOfBoxExperienceSettings) {
            $o = $i.outOfBoxExperienceSettings
            $oobe = "UserType: $($o.userType); HidePrivacy: $($o.hidePrivacySettings); HideEULA: $($o.hideEULA); SkipKeyboard: $($o.skipKeyboardSelectionPage)"
        }
        $type = ([string]$i.'@odata.type') -replace '#microsoft\.graph\.', ''
        $summary += New-SummaryItem -Name $i.displayName -Type $type -Platform 'Windows' `
            -Modified $i.lastModifiedDateTime -Assignments (ConvertTo-AssignmentSummary $i.assignments "Autopilot profile: $($i.displayName)") `
            -Extra ("Join: {0}; NameTemplate: {1}; OOBE: {2}" -f $(if ($type -match 'AzureADJoin' -or -not ($type -match 'Hybrid')) { 'Entra join' } else { 'Hybrid join' }), $i.deviceNameTemplate, $oobe)
    }
    Save-Json $items 'AutopilotProfiles'
    Add-Summary 'Autopilot Deployment Profiles' $summary `
        'Autopilot deployment profiles incl. all OOBE/onboarding screen options. Full detail: Json\AutopilotProfiles\.' @('Extra')
}

if (-not $SkipAutopilotDevices) {
    Invoke-Section 'Autopilot device identities (group tags)' {
        $items = Invoke-GraphGetAll "$GraphBase/deviceManagement/windowsAutopilotDeviceIdentities"
        Save-Json $items 'AutopilotDevices'
        # Summarize per group tag
        $byTag = $items | Group-Object -Property groupTag | Sort-Object Count -Descending
        $summary = foreach ($g in $byTag) {
            $tag = $g.Name; if ([string]::IsNullOrEmpty($tag)) { $tag = '(no group tag)' }
            New-SummaryItem -Name $tag -Type 'Group tag' -Platform 'Windows' -Modified '' -Assignments '-' -Extra ("{0} devices" -f $g.Count)
        }
        Add-Summary 'Autopilot Devices & Group Tags' $summary `
            ("Total Autopilot device identities: {0}. Full list incl. serial numbers, models and assigned profiles: Json\AutopilotDevices\_all.json." -f @($items).Count) @('Extra')
    }
}

Invoke-Section 'Enrollment configurations (ESP, restrictions, WHfB)' {
    $items = Invoke-GraphGetAll "$GraphBase/deviceManagement/deviceEnrollmentConfigurations"
    $summary = @()
    foreach ($i in $items) {
        $i | Add-Member -NotePropertyName 'assignments' -NotePropertyValue (Get-ItemAssignments "$GraphBase/deviceManagement/deviceEnrollmentConfigurations" $i.id) -Force
        Save-ItemJson $i 'EnrollmentConfigurations'
        $type = ([string]$i.'@odata.type') -replace '#microsoft\.graph\.', ''
        $extra = ''
        if ($type -eq 'windows10EnrollmentCompletionPageConfiguration') {
            $extra = "ESP - BlockUntilDone: $($i.showInstallationProgress); TimeoutMin: $($i.installProgressTimeoutInMinutes); AllowReset: $($i.allowDeviceResetOnInstallFailure)"
        }
        $summary += New-SummaryItem -Name $i.displayName -Type $type -Platform '' `
            -Modified $i.lastModifiedDateTime -Assignments (ConvertTo-AssignmentSummary $i.assignments "Enrollment config: $($i.displayName)") -Extra $extra
    }
    Save-Json $items 'EnrollmentConfigurations'
    Add-Summary 'Enrollment Configurations' $summary `
        'Enrollment Status Page (ESP), enrollment device platform restrictions, device limit restrictions, Windows Hello for Business, enrollment notifications. Priority order and full settings: Json\EnrollmentConfigurations\.' @('Extra')
}

Invoke-Section 'Apple enrollment (ADE/DEP, MDM push cert)' {
    $result = [ordered]@{}
    $summary = @()
    $apns = Invoke-GraphGet "$GraphBase/deviceManagement/applePushNotificationCertificate"
    if ($apns) {
        $result['applePushNotificationCertificate'] = $apns
        $summary += New-SummaryItem -Name 'Apple MDM Push Certificate' -Type 'APNs certificate' -Platform 'macOS/iOS' `
            -Modified '' -Assignments '-' -Extra ("AppleID: {0}; Expires: {1}" -f $apns.appleIdentifier, $apns.expirationDateTime)
    }
    $dep = @()
    try { $dep = Invoke-GraphGetAll "$GraphBase/deviceManagement/depOnboardingSettings" } catch {}
    $result['depOnboardingSettings'] = $dep
    foreach ($t in $dep) {
        $summary += New-SummaryItem -Name $t.tokenName -Type 'ADE/DEP token' -Platform 'macOS/iOS' `
            -Modified $t.lastModifiedDateTime -Assignments '-' -Extra ("AppleID: {0}; TokenExpires: {1}" -f $t.appleIdentifier, $t.tokenExpirationDateTime)
        $profiles = @()
        try { $profiles = Invoke-GraphGetAll "$GraphBase/deviceManagement/depOnboardingSettings/$($t.id)/enrollmentProfiles" } catch {}
        $result["enrollmentProfiles_$($t.id)"] = $profiles
        foreach ($p in $profiles) {
            $summary += New-SummaryItem -Name $p.displayName -Type 'ADE enrollment profile' -Platform (Get-PlatformFromODataType ([string]$p.'@odata.type')) `
                -Modified '' -Assignments '-' -Extra ("Default: {0}; Supervised/UserAuth see JSON" -f $p.isDefault)
        }
    }
    $uie = @()
    try { $uie = Invoke-GraphGetAll "$GraphBase/deviceManagement/appleUserInitiatedEnrollmentProfiles" } catch {}
    $result['appleUserInitiatedEnrollmentProfiles'] = $uie
    Save-Json $result 'AppleEnrollment'
    Add-Summary 'Apple Enrollment' $summary `
        'Apple MDM push certificate, Automated Device Enrollment (ADE/DEP) tokens and enrollment profiles. Full settings: Json\AppleEnrollment\.' @('Extra')
}

#endregion

#region ── 4. Intune: Applications ──────────────────────────────────────────────

Invoke-Section 'Applications (all Intune apps + assignments)' {
    $items = Invoke-GraphGetAll "$GraphBase/deviceAppManagement/mobileApps?`$expand=assignments"
    Save-Json $items 'Applications'
    $summary = @()
    foreach ($i in $items) {
        $type = ([string]$i.'@odata.type') -replace '#microsoft\.graph\.', ''
        $summary += New-SummaryItem -Name $i.displayName -Type $type -Platform (Get-PlatformFromODataType $type) `
            -Modified $i.lastModifiedDateTime -Assignments (ConvertTo-AssignmentSummary $i.assignments "App: $($i.displayName)") `
            -Extra ("Publisher: {0}; Version: {1}" -f $i.publisher, $(if ($i.PSObject.Properties.Name -contains 'displayVersion') { $i.displayVersion } else { '' }))
    }
    Add-Summary 'Applications' $summary `
        'All applications in Intune (Win32, LOB, Microsoft Store, macOS PKG/DMG/VPP, web links, M365 Apps) with assignment intent (required/available/uninstall). Detection rules, requirements and install commands: Json\Applications\_all.json.' @('Extra')
}

Invoke-Section 'App protection policies' {
    $result = [ordered]@{}
    $summary = @()
    $map = @(
        @{ Uri = "$GraphBase/deviceAppManagement/iosManagedAppProtections?`$expand=assignments";       Type = 'App protection (iOS)' },
        @{ Uri = "$GraphBase/deviceAppManagement/androidManagedAppProtections?`$expand=assignments";   Type = 'App protection (Android)' },
        @{ Uri = "$GraphBase/deviceAppManagement/windowsManagedAppProtections?`$expand=assignments";   Type = 'App protection (Windows)' },
        @{ Uri = "$GraphBase/deviceAppManagement/mdmWindowsInformationProtectionPolicies";             Type = 'WIP (MDM)' },
        @{ Uri = "$GraphBase/deviceAppManagement/windowsInformationProtectionPolicies";                Type = 'WIP (without enrollment)' }
    )
    foreach ($m in $map) {
        $items = @()
        try { $items = Invoke-GraphGetAll $m.Uri } catch { continue }
        $result[$m.Type] = $items
        foreach ($i in $items) {
            $asg = if ($i.PSObject.Properties.Name -contains 'assignments') { $i.assignments } else { @() }
            $summary += New-SummaryItem -Name $i.displayName -Type $m.Type -Platform '' `
                -Modified $i.lastModifiedDateTime -Assignments (ConvertTo-AssignmentSummary $asg "$($m.Type): $($i.displayName)")
        }
    }
    Save-Json $result 'AppProtectionPolicies'
    Add-Summary 'App Protection Policies' $summary 'MAM app protection policies. Full settings: Json\AppProtectionPolicies\.'
}

Invoke-Section 'App configuration policies' {
    $result = [ordered]@{}
    $summary = @()
    $mdm = @()
    try { $mdm = Invoke-GraphGetAll "$GraphBase/deviceAppManagement/mobileAppConfigurations?`$expand=assignments" } catch {}
    $result['managedDevices'] = $mdm
    foreach ($i in $mdm) {
        $summary += New-SummaryItem -Name $i.displayName -Type 'App config (managed devices)' -Platform '' `
            -Modified $i.lastModifiedDateTime -Assignments (ConvertTo-AssignmentSummary $i.assignments "App config: $($i.displayName)")
    }
    $mam = @()
    try { $mam = Invoke-GraphGetAll "$GraphBase/deviceAppManagement/targetedManagedAppConfigurations?`$expand=assignments" } catch {}
    $result['managedApps'] = $mam
    foreach ($i in $mam) {
        $summary += New-SummaryItem -Name $i.displayName -Type 'App config (managed apps)' -Platform '' `
            -Modified $i.lastModifiedDateTime -Assignments (ConvertTo-AssignmentSummary $i.assignments "App config (MAM): $($i.displayName)")
    }
    Save-Json $result 'AppConfigurationPolicies'
    Add-Summary 'App Configuration Policies' $summary 'App configuration for managed devices and managed apps. Full key/value payloads: Json\AppConfigurationPolicies\.'
}

#endregion

#region ── 5. Tenant / Intune admin objects ─────────────────────────────────────

Invoke-Section 'Intune tenant settings, categories, scope tags, roles, branding' {
    $tenant = Invoke-GraphGet "$GraphBase/deviceManagement?`$select=intuneAccountId,subscriptionState,settings,managedDeviceCleanupSettings,deviceComplianceCheckinThresholdDays"
    if ($tenant) { Save-Json $tenant 'TenantSettings' '_deviceManagement.json' }

    $summary = @()
    $cats = @(); try { $cats = Invoke-GraphGetAll "$GraphBase/deviceManagement/deviceCategories" } catch {}
    Save-Json $cats 'TenantSettings' '_deviceCategories.json'
    foreach ($c in $cats) { $summary += New-SummaryItem -Name $c.displayName -Type 'Device category' -Platform '' -Modified '' -Assignments '-' }

    $tags = @(); try { $tags = Invoke-GraphGetAll "$GraphBase/deviceManagement/roleScopeTags" } catch {}
    Save-Json $tags 'TenantSettings' '_roleScopeTags.json'
    foreach ($t in $tags) { $summary += New-SummaryItem -Name $t.displayName -Type 'Scope tag' -Platform '' -Modified '' -Assignments '-' -Extra ([string]$t.description) }

    $roles = @(); try { $roles = Invoke-GraphGetAll "$GraphBase/deviceManagement/roleDefinitions" } catch {}
    Save-Json $roles 'TenantSettings' '_roleDefinitions.json'
    foreach ($r in ($roles | Where-Object { -not $_.isBuiltIn })) {
        $summary += New-SummaryItem -Name $r.displayName -Type 'Custom Intune role' -Platform '' -Modified '' -Assignments '-' -Extra ([string]$r.description)
    }

    $tc = @(); try { $tc = Invoke-GraphGetAll "$GraphBase/deviceManagement/termsAndConditions" } catch {}
    Save-Json $tc 'TenantSettings' '_termsAndConditions.json'
    foreach ($t in $tc) { $summary += New-SummaryItem -Name $t.displayName -Type 'Terms & Conditions' -Platform '' -Modified $t.lastModifiedDateTime -Assignments '-' }

    $branding = @(); try { $branding = Invoke-GraphGetAll "$GraphBase/deviceManagement/intuneBrandingProfiles" } catch {}
    Save-Json $branding 'TenantSettings' '_intuneBrandingProfiles.json'
    foreach ($b in $branding) { $summary += New-SummaryItem -Name $b.profileName -Type 'Branding profile' -Platform '' -Modified $b.lastModifiedDateTime -Assignments '-' }

    $notif = @(); try { $notif = Invoke-GraphGetAll "$GraphBase/deviceManagement/notificationMessageTemplates" } catch {}
    Save-Json $notif 'TenantSettings' '_notificationMessageTemplates.json'
    foreach ($n in $notif) { $summary += New-SummaryItem -Name $n.displayName -Type 'Notification template' -Platform '' -Modified $n.lastModifiedDateTime -Assignments '-' }

    Add-Summary 'Intune Tenant Settings' $summary `
        'Device categories, scope tags, custom Intune RBAC roles, Terms & Conditions, Company Portal branding, compliance notification templates. Tenant-wide deviceManagement settings (incl. device cleanup rules): Json\TenantSettings\_deviceManagement.json.' @('Extra')
}

#endregion

#region ── 6. Entra ID: Conditional Access ──────────────────────────────────────

Invoke-Section 'Conditional Access policies' {
    $items = Invoke-GraphGetAll "$GraphBase/identity/conditionalAccess/policies"
    Save-Json $items 'ConditionalAccess'
    foreach ($i in $items) {
        Save-ItemJson $i 'ConditionalAccess'
        # collect referenced groups
        if ($i.conditions -and $i.conditions.users) {
            foreach ($gid in @($i.conditions.users.includeGroups)) { Add-GroupRef $gid "CA policy (include): $($i.displayName)" }
            foreach ($gid in @($i.conditions.users.excludeGroups)) { Add-GroupRef $gid "CA policy (exclude): $($i.displayName)" }
        }
    }
    $locations = @(); try { $locations = Invoke-GraphGetAll "$GraphBase/identity/conditionalAccess/namedLocations" } catch {}
    Save-Json $locations 'ConditionalAccess' '_namedLocations.json'
    $strengths = @(); try { $strengths = Invoke-GraphGetAll "$GraphBase/policies/authenticationStrengthPolicies" } catch {}
    Save-Json $strengths 'ConditionalAccess' '_authenticationStrengths.json'
    $agreements = @(); try { $agreements = Invoke-GraphGetAll "$GraphBase/identityGovernance/termsOfUse/agreements" } catch {}
    Save-Json $agreements 'ConditionalAccess' '_termsOfUseAgreements.json'

    $script:Counts['Conditional Access Policies'] = @($items).Count
    $script:Counts['CA Named Locations'] = @($locations).Count
    $script:Counts['CA Authentication Strengths'] = @($strengths).Count
}

#endregion

#region ── 7. Entra ID: Auth methods, SSPR, Sync ────────────────────────────────

Invoke-Section 'Authentication methods policy (MFA/SSPR methods)' {
    $amp = Invoke-GraphGet "$GraphV1/policies/authenticationMethodsPolicy"
    if ($amp) { Save-Json $amp 'AuthenticationMethods' }
    $script:Counts['Authentication Methods Policy'] = $(if ($amp) { 1 } else { 0 })
}

Invoke-Section 'Authorization policy & SSPR' {
    $authz = Invoke-GraphGet "$GraphBase/policies/authorizationPolicy"
    # beta returns a collection wrapper on this endpoint in some tenants; normalize
    if ($authz -and ($authz.PSObject.Properties.Name -contains 'value')) { $authz = @($authz.value)[0] }
    if ($authz) { Save-Json $authz 'AuthorizationPolicy' }
    $script:Counts['Authorization Policy / SSPR'] = $(if ($authz) { 1 } else { 0 })
}

Invoke-Section 'Entra Connect sync settings & organization' {
    $sync = @()
    try { $sync = Invoke-GraphGetAll "$GraphBase/directory/onPremisesSynchronization" } catch {}
    Save-Json $sync 'EntraSync' '_onPremisesSynchronization.json'
    $org = @()
    try { $org = Invoke-GraphGetAll "$GraphV1/organization" } catch {}
    Save-Json $org 'EntraSync' '_organization.json'
    $script:Counts['Entra Connect Sync'] = @($sync).Count
}

#endregion

#region ── 8. Groups (referenced + all dynamic groups) ──────────────────────────

Invoke-Section 'Resolving referenced groups + all dynamic groups' {
    $select = 'id,displayName,description,groupTypes,membershipRule,membershipRuleProcessingState,securityEnabled,mailEnabled,onPremisesSyncEnabled,createdDateTime'

    # 1) all dynamic groups in the tenant
    $dynamic = @()
    try {
        $uri = "$GraphBase/groups?`$filter=groupTypes/any(c:c eq 'DynamicMembership')&`$select=$select&`$count=true"
        $dynamic = Invoke-GraphGetAll -Uri $uri -Headers @{ ConsistencyLevel = 'eventual' }
    } catch { Write-Warning "Dynamic group query failed: $($_.Exception.Message)" }
    foreach ($g in $dynamic) { $script:GroupLookup[$g.id] = $g }
    Save-Json $dynamic 'Groups' '_allDynamicGroups.json'

    # 2) every group referenced in any assignment / CA policy
    foreach ($gid in $script:GroupIds) {
        if ($script:GroupLookup.ContainsKey($gid)) { continue }
        $g = Invoke-GraphGet "$GraphBase/groups/$gid`?`$select=$select"
        if ($g) { $script:GroupLookup[$gid] = $g }
    }
    $referenced = @()
    foreach ($gid in $script:GroupIds) {
        if ($script:GroupLookup.ContainsKey($gid)) {
            $g = $script:GroupLookup[$gid]
            $usage = @()
            if ($script:GroupUsage.ContainsKey($gid)) { $usage = @($script:GroupUsage[$gid]) }
            $g | Add-Member -NotePropertyName 'usedIn' -NotePropertyValue $usage -Force
            $referenced += $g
        } else {
            $referenced += [PSCustomObject]@{ id = $gid; displayName = '(unresolved / deleted)'; usedIn = @($script:GroupUsage[$gid]) }
        }
    }
    Save-Json $referenced 'Groups' '_referencedGroups.json'
    $script:Counts['Groups (referenced)'] = @($referenced).Count
    $script:Counts['Groups (dynamic, tenant-wide)'] = @($dynamic).Count
}

#endregion

#region ── 9. Export manifest (machine-readable index) ─────────────────────────

Write-Host "==> Writing export manifest" -ForegroundColor Cyan

$categories = [ordered]@{}
foreach ($title in $script:Summaries.Keys) {
    $data = $script:Summaries[$title]
    $items = @()
    foreach ($i in @($data.Items)) {
        $items += [pscustomobject]@{
            name         = $i.Name
            type         = $i.Type
            platform     = $i.Platform
            lastModified = $i.Modified
            assignments  = (Expand-GroupTokens ([string]$i.Assignments))
            details      = $i.Extra
        }
    }
    $categories[$title] = [pscustomobject]@{
        description = $data.Notes
        itemCount   = @($items).Count
        items       = $items
    }
}

$manifest = [pscustomobject]@{
    exportedAt  = (Get-Date).ToString('o')
    account     = $ctx.Account
    tenantId    = $ctx.TenantId
    counts      = $script:Counts
    categories  = $categories
    errors      = @($script:Errors)
    manualDocumentationRequired = @(
        'SSPR portal settings: scope (None/Selected/All), methods required to reset, reset notifications, helpdesk link - not exposed via Graph API',
        'Entra Connect server-side config: OU filtering, custom sync rules (run Get-ADSyncRule on the Connect server)',
        'Per-user legacy MFA states (if still used)',
        'Defender for Endpoint / partner connector status: Intune portal > Tenant administration > Connectors'
    )
}
ConvertTo-Json -InputObject $manifest -Depth 20 | Out-File -FilePath (Join-Path $JsonRoot '_exportManifest.json') -Encoding UTF8

#endregion

#region ── Done ─────────────────────────────────────────────────────────────────

Write-Host ''
Write-Host '──────────────────────────────────────────────' -ForegroundColor Green
Write-Host 'Export complete.' -ForegroundColor Green
Write-Host ("  Output:   {0}" -f $OutputPath)
Write-Host ("  JSON:     {0}" -f $JsonRoot)
Write-Host ("  Scripts:  {0}" -f $ScriptsRoot)
Write-Host ("  Manifest: {0}" -f (Join-Path $JsonRoot '_exportManifest.json'))
if ($script:Errors.Count -gt 0) {
    Write-Warning ("{0} section(s) reported errors - see 'errors' in Json\_exportManifest.json" -f $script:Errors.Count)
}
if ($DisconnectWhenDone) { Disconnect-MgGraph | Out-Null; Write-Host 'Graph session disconnected.' }
else { Write-Host 'Graph session left connected (use Disconnect-MgGraph to sign out).' }

#endregion
