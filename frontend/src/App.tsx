import { lazy, Suspense, useMemo, useState } from 'react';
import { Navigate, Route, Routes } from 'react-router-dom';
import { QueryClientProvider } from '@tanstack/react-query';
import { CssBaseline, StyledEngineProvider, ThemeProvider, useMediaQuery } from '@mui/material';
import { AuthProvider } from './contexts/AuthProvider';
import ProtectedRoute from './auth/ProtectedRoute';
import IndexRedirect from './auth/IndexRedirect';
import RouteFallback from './components/RouteFallback';
import LoginPage from './pages/LoginPage';
import CallbackPage from './pages/CallbackPage';
import NotFoundPage from './pages/NotFoundPage';
import { createAppTheme } from './theme';
import BaseLayout from './components/BaseLayout.tsx';
import { ApiProvider } from './contexts/ApiProvider.tsx';
import { createQueryClient } from './api/queryClient.ts';
import { ConfirmProvider } from './contexts/ConfirmProvider.tsx';
import { ROUTES, ROUTE_SEGMENTS } from './routes.ts';

// Route-level components are code-split so the login page loads without them.
const StreetsPage = lazy(() => import('./pages/StreetsPage.tsx'));
const StreetPage = lazy(() => import('./pages/StreetPage.tsx'));

export default function App() {
  const prefersDarkMode = useMediaQuery('(prefers-color-scheme: dark)', { noSsr: true });
  const [queryClient] = useState(createQueryClient);
  const theme = useMemo(
    () => createAppTheme(prefersDarkMode ? 'dark' : 'light'),
    [prefersDarkMode],
  );

  // injectFirst puts MUI's styles before the component .scss files in the
  // cascade, so a plain single-class selector there wins over MUI's defaults.
  return (
    <StyledEngineProvider injectFirst>
      <ThemeProvider theme={theme}>
        <CssBaseline />
        <QueryClientProvider client={queryClient}>
          <ConfirmProvider>
            <AuthProvider>
              <ApiProvider>
                <Suspense fallback={<RouteFallback />}>
                  <Routes>
                    <Route index element={<IndexRedirect />} />
                    <Route path={ROUTE_SEGMENTS.base}>
                      <Route path={ROUTE_SEGMENTS.login} element={<LoginPage />} />
                      <Route path={ROUTE_SEGMENTS.callback} element={<CallbackPage />} />
                      <Route element={<ProtectedRoute />}>
                        <Route element={<BaseLayout />}>
                          <Route index element={<Navigate to={ROUTES.streets} replace />} />
                          <Route path={ROUTE_SEGMENTS.streets}>
                            <Route index element={<StreetsPage />} />
                            <Route path=":streetId" element={<StreetPage />} />
                          </Route>
                          <Route path="*" element={<NotFoundPage />} />
                        </Route>
                      </Route>
                    </Route>
                    <Route path="*" element={<IndexRedirect />} />
                  </Routes>
                </Suspense>
              </ApiProvider>
            </AuthProvider>
          </ConfirmProvider>
        </QueryClientProvider>
      </ThemeProvider>
    </StyledEngineProvider>
  );
}
