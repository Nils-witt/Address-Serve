import { useEffect, useRef, useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { Alert, Box, Button, Stack } from '@mui/material';
import { useTranslation } from 'react-i18next';
import RouteFallback from '../components/RouteFallback';
import { useAuth } from '../hooks/useAuth';
import { errorMessage } from '../lib/errors';
import { ROUTES } from '../routes';

/** Where the OpenID provider sends the user back to: trades the
 * authorization code for tokens, then continues where they started. */
export default function CallbackPage() {
  const { t } = useTranslation();
  const { completeLogin } = useAuth();
  const navigate = useNavigate();
  const [error, setError] = useState<string | null>(null);
  // The code is single-use; StrictMode's second effect run must not redeem it again.
  const started = useRef(false);

  useEffect(() => {
    if (started.current) return;
    started.current = true;
    completeLogin()
      .then((from) => navigate(from ?? ROUTES.root, { replace: true }))
      .catch((err: unknown) => setError(errorMessage(err)));
  }, [completeLogin, navigate]);

  if (!error) return <RouteFallback />;
  return (
    <Box className="route-fallback">
      <Stack spacing={2}>
        <Alert severity="error">{t('login.callbackFailed', { reason: error })}</Alert>
        <Button component={Link} to={ROUTES.login} replace>
          {t('login.backToLogin')}
        </Button>
      </Stack>
    </Box>
  );
}
