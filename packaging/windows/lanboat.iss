[Setup]
AppId=org.lanboat.desktop
AppName=Lanboat
AppVersion={#Version}
DefaultDirName={localappdata}\Programs\Lanboat
DisableDirPage=no
PrivilegesRequired=lowest
ArchitecturesAllowed={#Architecture}
ArchitecturesInstallIn64BitMode={#Architecture}
OutputDir=..\..\dist
OutputBaseFilename=Lanboat-{#Version}-windows-{#CPU}-setup
Compression=lzma2/max
SolidCompression=yes
SetupIconFile=lanboat.ico
UninstallDisplayIcon={app}\lanboat.ico
[Files]
Source: "lanboat.ico"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\..\build\bin\Lanboat.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\..\build\notices\*"; DestDir: "{app}\notices"; Flags: recursesubdirs
[Icons]
Name: "{userprograms}\Lanboat"; Filename: "{app}\Lanboat.exe"; IconFilename: "{app}\lanboat.ico"
[Registry]
Root: HKCU; Subkey: "Software\Lanboat"; ValueType: string; ValueName: "DataDir"; ValueData: "{code:DataPath}"; Flags: uninsdeletevalue
[Code]
var DataPage: TInputDirWizardPage;
procedure InitializeWizard;
var Existing: String;
begin
  DataPage := CreateInputDirPage(wpSelectDir, 'User data folder',
    'Choose where to keep profiles, rules and logs.', '', False, '');
  DataPage.Add('Folder:');
  if not RegQueryStringValue(HKCU, 'Software\Lanboat', 'DataDir', Existing) then
    Existing := ExpandConstant('{%USERPROFILE}\.lanboat');
  DataPage.Values[0] := ExpandConstant('{param:DATADIR|' + Existing + '}');
end;
function DataPath(Param: String): String;
begin
  Result := DataPage.Values[0];
end;
