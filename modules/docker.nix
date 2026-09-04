{ ... }:

{
  # Each normal user gets an isolated daemon and socket without root access.
  virtualisation.docker.rootless = {
    enable = true;
    setSocketVariable = true;
  };
}
