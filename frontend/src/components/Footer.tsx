import { Box, Link, Typography } from '@mui/material';
import { useAuth } from '../hooks/useAuth';
import './Footer.scss';

export default function Footer() {
  const { config } = useAuth();
  const buildInfo = [config.version, config.commit].filter(Boolean).join(' · ');

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
