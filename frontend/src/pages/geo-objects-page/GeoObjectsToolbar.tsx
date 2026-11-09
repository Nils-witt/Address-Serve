import { Autocomplete, Button, TextField } from '@mui/material';
import AddIcon from '@mui/icons-material/Add';
import { useTranslation } from 'react-i18next';
import { useGeoObjectGroups } from '../../hooks/useGeoObjects';
import './GeoObjectsToolbar.scss';

export default function GeoObjectsToolbar({
  name,
  group,
  onNameChange,
  onGroupChange,
  onCreate,
}: {
  name: string;
  group: string;
  onNameChange: (name: string) => void;
  onGroupChange: (group: string) => void;
  onCreate: () => void;
}) {
  const { t } = useTranslation();
  const groups = useGeoObjectGroups();

  return (
    <div className="geo-objects-toolbar">
      <div className="geo-objects-toolbar__filters">
        <TextField
          label={t('geoObjects.searchLabel')}
          size="small"
          value={name}
          onChange={(e) => onNameChange(e.target.value)}
        />
        <Autocomplete
          options={groups}
          value={group || null}
          onChange={(_, value) => onGroupChange(value ?? '')}
          className="geo-objects-toolbar__select"
          renderInput={(params) => <TextField {...params} label={t('fields.group')} size="small" />}
        />
      </div>
      <Button variant="contained" startIcon={<AddIcon />} onClick={onCreate}>
        {t('geoObjects.newGeoObject')}
      </Button>
    </div>
  );
}
