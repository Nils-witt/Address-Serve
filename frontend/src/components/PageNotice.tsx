import { Link } from 'react-router-dom';
import { Button, Paper, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import './PageNotice.scss';
import { ROUTES } from '../routes';

/** A page-sized message with a way back: used where a route can't show its
 * content (street missing, page not found). */
export default function PageNotice({ message }: { message: string }) {
  const { t } = useTranslation();
  return (
    <Paper className="page-notice">
      <Typography variant="body2" color="text.secondary">
        {message}
      </Typography>
      <Button
        component={Link}
        to={ROUTES.streets}
        size="small"
        className="page-notice__back-button"
      >
        {t('common.backToStreets')}
      </Button>
    </Paper>
  );
}
