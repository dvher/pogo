#!/bin/sh

# Update desktop database for .desktop file changes
# This makes the application appear in application menus and registers its capabilities.
if command -v update-desktop-database >/dev/null 2>&1; then
  echo "Updating desktop database..."
  update-desktop-database -q /usr/share/applications
else
  echo "Warning: update-desktop-database command not found. Desktop file may not be immediately recognized." >&2
fi

# Update MIME database for custom URL schemes (x-scheme-handler)
# This ensures the system knows how to handle your custom protocols.
if command -v update-mime-database >/dev/null 2>&1; then
  echo "Updating MIME database..."
  update-mime-database -n /usr/share/mime
else
  echo "Warning: update-mime-database command not found. Custom URL schemes may not be immediately recognized." >&2
fi

# Load the AppArmor profile that lets WebKit's sandbox run (Ubuntu 24.04+)
if [ -f /etc/apparmor.d/pogo ] && command -v apparmor_parser >/dev/null 2>&1 \
  && [ -d /sys/kernel/security/apparmor ]; then
  echo "Loading AppArmor profile..."
  apparmor_parser -r -T -W /etc/apparmor.d/pogo || \
    echo "Warning: could not load /etc/apparmor.d/pogo; Pogo may fail to start." >&2
fi

exit 0
