'use strict';
(function () {
  const version = 'v0.8.1';
  function installation(os, arch) {
    if (!['linux', 'windows', 'macos'].includes(os) || !['amd64', 'arm64'].includes(arch)) {
      throw new Error('Choose a supported operating system and processor.');
    }
    if (os === 'macos') return {url: null, command: `go run github.com/Alifarid0011/gobook@${version}`};
    const folder = `gobook-${version}-${os}-${arch}`;
    const archive = folder + (os === 'linux' ? '.tar.gz' : '.zip');
    const url = `https://github.com/Alifarid0011/gobook/releases/download/${version}/${archive}`;
    const command = os === 'linux'
      ? `sha256sum -c ${archive}.sha256\ntar -xzf ${archive}\ncd ${folder}\n./install.sh`
      : `Get-FileHash .\\${archive} -Algorithm SHA256\nGet-Content .\\${archive}.sha256\n# Compare the hashes before continuing.\nExpand-Archive .\\${archive} -DestinationPath .\ncd .\\${folder}\n.\\Install-GoBook.ps1`;
    return {url, command};
  }
  if (typeof module !== 'undefined') module.exports = {installation};
  if (typeof document === 'undefined') return;
  const $ = id => document.getElementById(id);
  function update() {
    const os = $('platform').value;
    const result = installation(os, $('processor').value);
    $('processor').disabled = os === 'macos';
    $('setup-command').textContent = result.command;
    $('archive-links').hidden = !result.url;
    if (result.url) {
      $('archive-download').href = result.url;
      $('checksum-download').href = result.url + '.sha256';
      $('archive-download').textContent = `Download for ${os === 'linux' ? 'Linux' : 'Windows'} ${$('processor').value === 'amd64' ? 'x64' : 'ARM64'}`;
    }
    $('setup-note').textContent = os === 'macos'
      ? 'macOS uses the Go command below. There is no packaged macOS desktop installer; local macOS runtime behavior has not been verified.'
      : 'Download both files into the same folder. Verify the checksum, then run these commands there. The installer adds GoBook Local to your application menu.';
    $('copy-setup').textContent = 'Copy commands';
  }
  $('platform').addEventListener('change', update);
  $('processor').addEventListener('change', update);
  $('copy-setup').addEventListener('click', async () => {
    try {
      await navigator.clipboard.writeText($('setup-command').textContent);
      $('copy-setup').textContent = 'Copied';
    } catch (_) {
      $('copy-setup').textContent = 'Select the commands to copy';
    }
  });
  update();
})();
