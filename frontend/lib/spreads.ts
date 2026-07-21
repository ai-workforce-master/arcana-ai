export interface Spread {
  id: string;
  name: string;
  cardCount: number;
  price: number;
  description: string;
  positions: string[];
}

export const spreads: Spread[] = [
  { id: 'one-card', name: 'Одна карта (Карта дня)', cardCount: 1, price: 50, description: 'Послание дня, основная энергия или тема', positions: ['Послание дня'] },
  { id: 'yes-no', name: 'Да или Нет (3 карты)', cardCount: 3, price: 100, description: 'Быстрый ответ на вопрос с анализом факторов', positions: ['Факторы ДА', 'Факторы НЕТ', 'Итог'] },
  { id: 'triplet', name: 'Три карты (Триплет)', cardCount: 3, price: 150, description: 'Прошлое — Настоящее — Будущее', positions: ['Прошлое', 'Настоящее', 'Будущее'] },
  { id: 'cross', name: 'Крест (4 карты)', cardCount: 4, price: 200, description: 'Баланс сил в ситуации', positions: ['Центр', 'Слева (препятствия)', 'Справа (поддержка)', 'Сверху (итог)'] },
  { id: 'five-cards', name: 'Пятикарточный расклад', cardCount: 5, price: 250, description: 'Детальный анализ проблемы с советом', positions: ['Прошлое', 'Настоящее', 'Скрытые влияния', 'Совет', 'Результат'] },
  { id: 'horseshoe', name: 'Подкова (7 карт)', cardCount: 7, price: 300, description: 'Комплексный анализ важного вопроса', positions: ['Прошлое', 'Настоящее', 'Скрытые влияния', 'Препятствия', 'Окружение', 'Совет', 'Результат'] },
  { id: 'celtic-cross', name: 'Кельтский крест (10 карт)', cardCount: 10, price: 500, description: 'Самый полный и универсальный расклад', positions: ['Ситуация', 'Препятствие', 'Далекое прошлое', 'Недавнее прошлое', 'Возможное будущее', 'Ближайшее будущее', 'Ваше состояние', 'Окружение', 'Надежды и страхи', 'Результат'] },
];

export function getSpreadById(id: string): Spread | undefined {
  return spreads.find(s => s.id === id);
}
