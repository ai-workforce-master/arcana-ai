export interface Spread {
  id: string;
  name: string;
  cardCount: number;
  price: number;
  description: string;
  positions: string[];
  category: 'simple' | 'medium' | 'complex';
}

export const spreads: Spread[] = [
  // === ПРОСТЫЕ ===
  {
    id: 'one-card',
    name: 'Одна карта (Карта дня)',
    cardCount: 1,
    price: 29,
    description: 'Послание дня, основная энергия или тема',
    positions: ['Послание дня'],
    category: 'simple',
  },
  {
    id: 'yes-no',
    name: 'Да или Нет (3 карты)',
    cardCount: 3,
    price: 69,
    description: 'Быстрый ответ на вопрос с анализом факторов',
    positions: ['Факторы ДА', 'Факторы НЕТ', 'Итог'],
    category: 'simple',
  },
  {
    id: 'triplet',
    name: 'Три карты (Триплет)',
    cardCount: 3,
    price: 69,
    description: 'Прошлое — Настоящее — Будущее',
    positions: ['Прошлое', 'Настоящее', 'Будущее'],
    category: 'simple',
  },
  {
    id: 'cross',
    name: 'Крест (4 карты)',
    cardCount: 4,
    price: 89,
    description: 'Баланс сил в ситуации',
    positions: ['Центр (суть)', 'Слева (препятствия)', 'Справа (поддержка)', 'Сверху (итог)'],
    category: 'simple',
  },
  
  // === СРЕДНИЕ ===
  {
    id: 'blind-spot',
    name: 'Слепое пятно (4 карты)',
    cardCount: 4,
    price: 89,
    description: 'Анализ личности через окно Джохари',
    positions: ['Осознаваемое', 'Мистер Икс (скрытое)', 'Тень', 'Слепое пятно'],
    category: 'medium',
  },
  {
    id: 'guardian-angel',
    name: 'Голос Ангела-Хранителя (4 карты)',
    cardCount: 4,
    price: 89,
    description: 'Духовная поддержка и наставление',
    positions: ['Истоки', 'Текущее состояние', 'Совет', 'Наставление ангела'],
    category: 'medium',
  },
  {
    id: 'five-cards',
    name: 'Пятикарточный расклад',
    cardCount: 5,
    price: 109,
    description: 'Детальный анализ проблемы с советом',
    positions: ['Прошлое', 'Настоящее', 'Скрытые влияния', 'Совет', 'Результат'],
    category: 'medium',
  },
  {
    id: 'pyramid',
    name: 'Пирамида (6 карт)',
    cardCount: 6,
    price: 129,
    description: 'Анализ развития проекта или ситуации',
    positions: ['Прошлые влияния', 'Текущая ситуация', 'Внешние факторы', 'Внутренние процессы', 'Вызовы', 'Вершина (итог)'],
    category: 'medium',
  },
  {
    id: 'alchemist',
    name: 'Алхимик (6 карт)',
    cardCount: 6,
    price: 129,
    description: 'Взгляд в будущее через временную перспективу',
    positions: ['Прошлое', 'Настоящее', 'Будущее', 'Ближайшее будущее', 'Промежуточное', 'Отдаленное'],
    category: 'medium',
  },
  {
    id: 'horseshoe',
    name: 'Подкова (7 карт)',
    cardCount: 7,
    price: 149,
    description: 'Комплексный анализ важного вопроса',
    positions: ['Прошлое', 'Настоящее', 'Скрытые влияния', 'Препятствия', 'Окружение', 'Совет', 'Результат'],
    category: 'medium',
  },
  {
    id: 'star',
    name: 'Звезда / Чакральный (7 карт)',
    cardCount: 7,
    price: 149,
    description: 'Анализ энергетического баланса по чакрам',
    positions: ['Корневая чакра', 'Сакральная', 'Солнечное сплетение', 'Сердечная', 'Горловая', 'Третий глаз', 'Коронная'],
    category: 'medium',
  },
  {
    id: 'partnership',
    name: 'Партнерство (7 карт)',
    cardCount: 7,
    price: 149,
    description: 'Глубокий анализ отношений',
    positions: ['Сигнификатор', 'Сознание спрашивающего', 'Эмоции спрашивающего', 'Внешняя сторона (спрашивающий)', 'Сознание партнера', 'Эмоции партнера', 'Внешняя сторона (партнер)'],
    category: 'medium',
  },
  
  // === СЛОЖНЫЕ ===
  {
    id: 'celtic-cross',
    name: 'Кельтский крест (10 карт)',
    cardCount: 10,
    price: 199,
    description: 'Самый полный и универсальный расклад',
    positions: ['Ситуация', 'Препятствие', 'Далекое прошлое', 'Недавнее прошлое', 'Возможное будущее', 'Ближайшее будущее', 'Ваше состояние', 'Окружение', 'Надежды и страхи', 'Результат'],
    category: 'complex',
  },
  {
    id: 'wheel-of-year',
    name: 'Колесо года (12 карт)',
    cardCount: 12,
    price: 229,
    description: 'Прогноз на каждый месяц года',
    positions: ['Январь', 'Февраль', 'Март', 'Апрель', 'Май', 'Июнь', 'Июль', 'Август', 'Сентябрь', 'Октябрь', 'Ноябрь', 'Декабрь'],
    category: 'complex',
  },
];

export function getSpreadById(id: string): Spread | undefined {
  return spreads.find(s => s.id === id);
}

export const categoryNames: Record<string, string> = {
  simple: '🌱 Простые расклады',
  medium: '🔮 Средней сложности',
  complex: '⭐ Сложные расклады',
};
