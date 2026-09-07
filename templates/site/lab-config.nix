{
  masterDhcpIp = "MASTER_DHCP_IP";
  networkBase = "10.0.0";
  pcCount = 20;
  masterHostNumber = 99;
  ifaceName = "enp0s3";

  teacherUser = "teacher";
  studentUser = "student";

  # Generate each hash with: mkpasswd -m sha-512
  teacherPassword = "$6$t.4PBRDwSMnGbuzA$fLuu1n700q.Mvj0ivauGLPQJcfT6XnFMkDh6T0GMWH/hzlSNuzxfh0bxh2iQR027y7PSdzuIvWoO3NgRbM/gV0";
  studentPassword = "$6$t.4PBRDwSMnGbuzA$fLuu1n700q.Mvj0ivauGLPQJcfT6XnFMkDh6T0GMWH/hzlSNuzxfh0bxh2iQR027y7PSdzuIvWoO3NgRbM/gV0";
  adminPassword = "$6$t.4PBRDwSMnGbuzA$fLuu1n700q.Mvj0ivauGLPQJcfT6XnFMkDh6T0GMWH/hzlSNuzxfh0bxh2iQR027y7PSdzuIvWoO3NgRbM/gV0";

  homepageUrl = "https://example.org";
  studentGitName = "student";
  studentGitEmail = "student@example.org";
  adminGitName = "admin";
  adminGitEmail = "admin@example.org";

  timeZone = "Europe/Rome";
  defaultLocale = "en_US.UTF-8";
  extraLocale = "it_IT.UTF-8";
  keyboardLayout = "it";
  consoleKeyMap = "it2";

  veyonNativeHosts = [];
}
