import { useEffect, useState } from 'react';
import { Navigate, useLocation } from 'react-router-dom';
import { Alert, Box, Button, Container, Paper, Stack, Typography } from '@mui/material';
import LoginIcon from '@mui/icons-material/Login';
import { useTranslation } from 'react-i18next';
import { useAuth } from '../hooks/useAuth';
import Footer from '../components/Footer';
import LanguageSwitcher from '../components/LanguageSwitcher';
import { errorMessage } from '../lib/errors';
import './LoginPage.scss';
import { ROUTES } from '../routes';

export default function LoginPage() {
  const { t } = useTranslation();
  const auth = useAuth();
  const routerLocation = useLocation();
  const from = (routerLocation.state as { from?: string } | null)?.from ?? null;

  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (auth.sessionMessage) {
      setError(auth.sessionMessage);
      auth.clearSessionMessage();
    }
    // Only re-run when the session-expired message actually changes.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [auth.sessionMessage]);

  if (auth.isAuthenticated) return <Navigate to={from ?? ROUTES.root} replace />;

  const handleSignIn = async () => {
    setError(null);
    setSubmitting(true);
    try {
      // Leaves the page on success.
      await auth.login(from);
    } catch (err) {
      setError(t('login.loginFailed', { reason: errorMessage(err) }));
      setSubmitting(false);
    }
  };

  return (
    <Box className="login-page">
      <Box className="login-page__language-switcher">
        <LanguageSwitcher />
      </Box>
      <Container maxWidth="xs" disableGutters>
        <Paper elevation={3}>
          <Stack spacing={2}>
            <Typography variant="h5" component="h1" className="login-page__title">
              {t('nav.brand')}
            </Typography>
            <Typography variant="body2" color="text.secondary" className="login-page__title">
              {t('login.intro')}
            </Typography>
            <Button
              variant="contained"
              fullWidth
              disabled={submitting}
              startIcon={<LoginIcon />}
              onClick={() => void handleSignIn()}
            >
              {submitting ? t('login.signingIn') : t('login.signInWithSso')}
            </Button>
          </Stack>
          {error && <Alert severity="error">{error}</Alert>}
        </Paper>
      </Container>
      <Footer />
    </Box>
  );
}
