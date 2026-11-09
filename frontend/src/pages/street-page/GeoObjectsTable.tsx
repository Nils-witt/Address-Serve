import {
  Button,
  IconButton,
  Link,
  Paper,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Tooltip,
  Typography,
} from '@mui/material';
import AddIcon from '@mui/icons-material/Add';
import DeleteIcon from '@mui/icons-material/Delete';
import EditIcon from '@mui/icons-material/Edit';
import { useTranslation } from 'react-i18next';
import type { GeoObject, HouseNumber } from '../../api/types';
import { formatCoordinates, formatHouseNumber, osmUrl } from '../../lib/format';
import './GeoObjectsTable.scss';

/** The geo objects on one street. */
export default function GeoObjectsTable({
  geoObjects,
  houseNumbersById,
  loading,
  onCreate,
  onEdit,
  onDelete,
}: {
  /** Already in display order. */
  geoObjects: GeoObject[];
  /** The street's house numbers, to label the geo objects at them. */
  houseNumbersById: Map<string, HouseNumber>;
  loading: boolean;
  onCreate: () => void;
  onEdit: (g: GeoObject) => void;
  onDelete: (g: GeoObject) => void;
}) {
  const { t } = useTranslation();
  return (
    <Paper className="geo-objects-table">
      <div className="geo-objects-table__header">
        <Typography variant="h6" component="h2">
          {t('geoObjects.title', { count: geoObjects.length })}
        </Typography>
        <Button variant="contained" startIcon={<AddIcon />} onClick={onCreate}>
          {t('geoObjects.newGeoObject')}
        </Button>
      </div>
      <TableContainer>
        <Table size="small">
          <TableHead>
            <TableRow>
              <TableCell>{t('fields.name')}</TableCell>
              <TableCell>{t('fields.group')}</TableCell>
              <TableCell>{t('fields.houseNumber')}</TableCell>
              <TableCell>{t('fields.coordinates')}</TableCell>
              <TableCell align="right">{t('common.actions')}</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {geoObjects.map((g) => {
              const houseNumber = g.houseNumberId
                ? houseNumbersById.get(g.houseNumberId)
                : undefined;
              return (
                <TableRow key={g.id} hover>
                  <TableCell>{g.name}</TableCell>
                  <TableCell>{g.group}</TableCell>
                  <TableCell>{houseNumber ? formatHouseNumber(houseNumber) : ''}</TableCell>
                  <TableCell className="geo-objects-table__coordinates">
                    <Link
                      href={osmUrl(g.latitude, g.longitude)}
                      target="_blank"
                      rel="noopener noreferrer"
                    >
                      {formatCoordinates(g.latitude, g.longitude)}
                    </Link>
                  </TableCell>
                  <TableCell align="right">
                    <Tooltip title={t('common.edit')}>
                      <IconButton size="small" onClick={() => onEdit(g)}>
                        <EditIcon fontSize="small" />
                      </IconButton>
                    </Tooltip>
                    <Tooltip title={t('common.delete')}>
                      <IconButton size="small" color="error" onClick={() => onDelete(g)}>
                        <DeleteIcon fontSize="small" />
                      </IconButton>
                    </Tooltip>
                  </TableCell>
                </TableRow>
              );
            })}
          </TableBody>
        </Table>
      </TableContainer>
      {!loading && geoObjects.length === 0 && (
        <Typography variant="body2" color="text.secondary" className="geo-objects-table__empty">
          {t('geoObjects.empty')}
        </Typography>
      )}
    </Paper>
  );
}
