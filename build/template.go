package main

const (
	preInstall = `#!/bin/sh

KITE_PLIST="/Library/LaunchAgents/com.koding.kite.{{.}}.plist"

# see: https://lists.macosforge.org/pipermail/launchd-dev/2011-January/000890.html
echo "Checking to unload plist"
for pid_uid in $(ps -axo pid,uid,args | grep -i "[l]oginwindow.app" | awk '{print $1 "," $2}'); do
    pid=$(echo $pid_uid | cut -d, -f1)
    uid=$(echo $pid_uid | cut -d, -f2)
    echo "unloading launch agent"
    launchctl bsexec "$pid" chroot -u "$uid" / launchctl unload ${KITE_PLIST}
done

KDFILE=/usr/local/bin/{{.}}

echo "Removing previous installation"
if [ -f $KDFILE  ]; then
    rm -r $KDFILE
fi

exit 0
`
	postInstall = `#!/bin/bash

KITE_PLIST="/Library/LaunchAgents/com.koding.kite.{{.}}.plist"
chown root:wheel ${KITE_PLIST}
chmod 644 ${KITE_PLIST}

# this is simpler than below, but it doesn't get the USER env always, don't know why.
# echo $USER
# su $USER -c "/bin/launchctl load ${KITE_PLIST}"

# see: https://lists.macosforge.org/pipermail/launchd-dev/2011-January/000890.html
echo "running postinstall actions for all logged in users."
for pid_uid in $(ps -axo pid,uid,args | grep -i "[l]oginwindow.app" | awk '{print $1 "," $2}'); do
    pid=$(echo $pid_uid | cut -d, -f1)
    uid=$(echo $pid_uid | cut -d, -f2)
    echo "loading launch agent"
    launchctl bsexec "$pid" chroot -u "$uid" / launchctl load ${KITE_PLIST}
done

exit 0
`

	distribution = `<?xml version="1.0" encoding="utf-8" standalone="no"?>
<installer-script minSpecVersion="1.000000">
    <title>Koding Kite</title>
    <background mime-type="image/png" file="bg.png"/>
    <options customize="never" allow-external-scripts="no"/>
    <!-- <domains enable_localSystem="true" /> -->
    <options rootVolumeOnly="true" />
    <installation-check script="installCheck();"/>
    <script>
function installCheck() {
    if(system.files.fileExistsAtPath('/usr/local/bin/{{.}}')) {
        my.result.title = 'Previous Installation Detected';
        my.result.message = 'A previous installation of Koding {{.}} Kite exists at /usr/local/bin. This installer will remove the previous installation prior to installing. Please back up any data before proceeding.';
        my.result.type = 'Warning';
        return false;
    }
    return true;
}
    </script>
    <!-- List all component packages -->
    <pkg-ref
        id="com.koding.kite.{{.}}.pkg"
        auth="root">com.koding.kite.{{.}}.pkg</pkg-ref>
    <choices-outline>
        <line choice="com.koding.kite.{{.}}.choice"/>
    </choices-outline>
    <choice
        id="com.koding.kite.{{.}}.choice"
        title="Koding Kite"
        customLocation="/">
        <pkg-ref id="com.koding.kite.{{.}}.pkg"/>
    </choice>
</installer-script>
`

	launchAgent = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>KeepAlive</key>
    <dict>
        <key>NetworkState</key>
        <true/>
    </dict>
    <key>Label</key>
    <string>com.koding.kite.{{.}}</string>
    <key>ProgramArguments</key>
    <array>
        <string>/usr/local/bin/{{.}}</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
</dict>
</plist>
`
)
