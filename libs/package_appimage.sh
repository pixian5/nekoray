#!/bin/bash

sudo apt-get install fuse -y

cp -r linux64 newbeeplus.AppDir

# The file for Appimage

rm newbeeplus.AppDir/launcher

cat >newbeeplus.AppDir/newbeeplus.desktop <<-EOF
[Desktop Entry]
Name=newbeeplus
Exec=echo "newbeeplus started"
Icon=newbeeplus
Type=Application
Categories=Network
EOF

cat >newbeeplus.AppDir/AppRun <<-EOF
#!/bin/bash
echo "PATH: \${PATH}"
echo "newbeeplus runing on: \$APPDIR"
LD_LIBRARY_PATH=\${APPDIR}/usr/lib QT_PLUGIN_PATH=\${APPDIR}/usr/plugins \${APPDIR}/newbeeplus -appdata "\$@"
EOF

chmod +x newbeeplus.AppDir/AppRun

# build

curl -fLSO https://github.com/AppImage/AppImageKit/releases/latest/download/appimagetool-x86_64.AppImage
chmod +x appimagetool-x86_64.AppImage
./appimagetool-x86_64.AppImage newbeeplus.AppDir

# clean

rm appimagetool-x86_64.AppImage
rm -rf newbeeplus.AppDir
