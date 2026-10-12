# No client interface name is configured, and the template fallback (enp0s3)
# never exists in the installer VMs. Rename the installed system's only NIC so
# that its name matches nothing the configuration knows.
{
  services.udev.extraRules = ''
    SUBSYSTEM=="net", ACTION=="add", KERNEL=="eth0", NAME="eno7"
  '';
}
