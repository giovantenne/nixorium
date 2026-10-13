{ pkgs, mkWorkspaceLab }:
let
  # This caller has the older argument shape: no homeFilesJSON/homeFilesRoot.
  lab = mkWorkspaceLab {
    deploymentSelf = ./fixtures/home-files-deployment;
    workspaceProfileJSON = ''{"schemaVersion":1}'';
    workspaceCatalog = { schemaVersion = 1; baseline.schemaVersion = 1; applications = []; extensions = []; };
    labSoftware = { schemaVersion = 1; packages = []; };
  };
  declared = builtins.fromJSON (builtins.readFile ./fixtures/home-files-deployment/home-files.json);
in
assert lab.nixoriumHomeFiles.declared == declared;
assert lab.nixoriumHomeFiles.targets == [
  { name = "pc99"; role = "controller"; }
  { name = "pc01"; role = "client"; }
  { name = "pc02"; role = "client"; }
];
assert lab.nixoriumValidateHomeFilesCandidate (builtins.toJSON declared);
assert !(builtins.tryEval (lab.nixoriumValidateHomeFilesCandidate ''{"schemaVersion":1,"unknown":true}'')).success;
pkgs.runCommand "nixorium-home-files-adoption-check" {} ''
  cmp ${./fixtures/home-files-deployment/home-files/imports/0e65a074a920df375483f2139a86aad84b3932e03a5f08f013ef142de1d008a9} \
    ${lab.nixoriumHomeFiles.seed}/home/Documents/lesson.bin
  grep -qx 'Adopted by the existing deployment.' ${lab.nixoriumHomeFiles.seed}/home/Documents/start.txt
  # The installer serializes the adopted declaration and captured source root;
  # its own offline evaluation must reproduce the original client derivations.
  test -e ${lab.nixoriumOfflineCheck}
  touch "$out"
''
