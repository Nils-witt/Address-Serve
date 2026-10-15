import { Box, Link, Typography } from '@mui/material';
import { useAuth } from '../hooks/useAuth';
import './Footer.scss';

// Placeholders the server reports when a build wasn't stamped with this info.
const UNTAGGED_VERSION = 'dev';
const UNKNOWN_COMMIT = 'none';

/** Release builds show their version; untagged builds show branch @ commit. */
function formatBuildInfo(version: string, commit: string, branch: string | undefined): string {
  const shortCommit = commit && commit !== UNKNOWN_COMMIT ? commit.slice(0, 7) : '';
  if (version && version !== UNTAGGED_VERSION) {
    return [version, shortCommit].filter(Boolean).join(' · ');
  }
  return [branch, shortCommit].filter(Boolean).join(' @ ') || version;
}

export default function Footer() {
  const { config } = useAuth();
  const buildInfo = formatBuildInfo(config.version, config.commit, config.branch);

  return (
    <Box component="footer" className="footer">
      <Typography variant="caption" color="text.secondary">
        &copy; 2026 Nils Witt &middot; Address Serve &middot;{' '}
        <Link
          href="https://github.com/Nils-witt/Address-Serve"
          target="_blank"
          rel="noopener noreferrer"
          color="inherit"
        >
          GitHub
        </Link>
        {buildInfo && ` · ${buildInfo}`}
      </Typography>
    </Box>
  );
}
